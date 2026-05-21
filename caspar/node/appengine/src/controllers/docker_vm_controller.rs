use crate::prelude::*;
use crate::controllers::vm_controller::VmController;
use crate::network::vm_network::VmNetworkService;
use crate::models::vm_runtime::parse_vm_resource_limits;
use crate::bridge::messaging::{wasm_send, log};
use crate::globals::GLOBAL_VM_CONTEXT;

pub(crate) struct DockerVmController {
    docker: Docker,
}

impl DockerVmController {
    pub(crate) fn new() -> Result<Self, String> {
        Docker::connect_with_local_defaults()
            .map(|docker| Self { docker })
            .map_err(|e| format!("docker client init failed: {}", e))
    }

    fn with_async<T, F>(&self, fut: F) -> Result<T, String>
    where
        F: std::future::Future<Output = Result<T, BollardError>>,
    {
        tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|e| format!("tokio runtime init failed: {}", e))?
            .block_on(fut)
            .map_err(|e| format!("docker api error: {}", e))
    }

    pub(crate) fn run_vm(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        let creature_id = packet["creatureId"]
            .as_str()
            .or_else(|| packet["userId"].as_str())
            .unwrap_or("")
            .to_string();
        let (entity_id, container_name, _standalone, vm_id) = extract_docker_identity(packet);
        let vm_cache_key = if vm_id.is_empty() {
            "main".to_string()
        } else {
            vm_id.clone()
        };
        let container_id = docker_container_id(machine_id, &entity_id, &container_name, &vm_id);
        let image_ref = packet["imageRef"]
            .as_str()
            .map(|s| s.to_string())
            .unwrap_or_else(|| docker_image_ref(machine_id, &entity_id));
        let limits = parse_vm_resource_limits(packet);

        self.stop_and_remove_if_exists(&container_id)?;

        let env = packet["env"].as_array().map(|v| {
            v.iter()
                .filter_map(|x| x.as_str().map(|s| s.to_string()))
                .collect::<Vec<String>>()
        });
        let cmd = packet["command"]
            .as_str()
            .map(|command| vec!["sh".to_string(), "-lc".to_string(), command.to_string()]);

        self.with_async(self.docker.create_container(
            Some(CreateContainerOptions {
                name: container_id.clone(),
                platform: None,
            }),
            DockerConfig {
                image: Some(image_ref),
                env,
                cmd,
                host_config: Some(HostConfig {
                    runtime: Some("runsc".to_string()),
                    network_mode: Some(VmNetworkService::gateway_network_name().to_string()),
                    memory: Some((limits.ram_mb * 1024 * 1024) as i64),
                    cpu_count: Some(limits.cpu_cores as i64),
                    storage_opt: Some(HashMap::from([(
                        "size".to_string(),
                        format!("{}G", limits.disk_gb),
                    )])),
                    ..Default::default()
                }),
                ..Default::default()
            },
        ))?;

        if !packet["inputFiles"].is_null() {
            let files = parse_input_files(&packet["inputFiles"])?;
            self.upload_files(&container_id, "/app/input", &files)?;
        }

        self.with_async(
            self.docker
                .start_container::<String>(&container_id, None::<StartContainerOptions<String>>),
        )?;
        let timeout_container = container_id.clone();
        let timeout_vm = vm_cache_key.clone();
        let timeout_secs = limits.max_exec_time_secs;
        thread::spawn(move || {
            thread::sleep(Duration::from_secs(timeout_secs));
            if let Ok(docker) = Docker::connect_with_local_defaults() {
                let rt = tokio::runtime::Builder::new_current_thread()
                    .enable_all()
                    .build();
                if let Ok(rt) = rt {
                    let _ = rt.block_on(docker.stop_container(
                        &timeout_container,
                        Some(StopContainerOptions { t: 1 }),
                    ));
                    let _ = rt.block_on(docker.remove_container(
                        &timeout_container,
                        Some(RemoveContainerOptions {
                            force: true,
                            v: true,
                            ..Default::default()
                        }),
                    ));
                }
            }
            let mut vm_ctx = GLOBAL_VM_CONTEXT.lock().unwrap();
            vm_ctx.remove(timeout_vm.as_str());
        });
        let container_id_for_logs = container_id.clone();
        let vm_id_for_logs = vm_cache_key.clone();
        thread::spawn(move || {
            let docker = match Docker::connect_with_local_defaults() {
                Ok(d) => d,
                Err(_) => return,
            };
            let rt = match tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
            {
                Ok(r) => r,
                Err(_) => return,
            };
            rt.block_on(async move {
                let mut stream = docker.logs(
                    &container_id_for_logs,
                    Some(LogsOptions::<String> {
                        follow: true,
                        stdout: true,
                        stderr: true,
                        tail: "0".to_string(),
                        ..Default::default()
                    }),
                );
                while let Some(msg) = stream.try_next().await.unwrap_or(None) {
                    match msg {
                        LogOutput::StdOut { message }
                        | LogOutput::StdErr { message }
                        | LogOutput::Console { message }
                        | LogOutput::StdIn { message } => {
                            let line = String::from_utf8_lossy(&message).to_string();
                            emit_vm_log(&vm_id_for_logs, "runtime", line.trim());
                        }
                    }
                }
            });
        });
        let mut vm_ctx = GLOBAL_VM_CONTEXT.lock().unwrap();
        vm_ctx.insert(vm_cache_key.clone(), (creature_id, machine_id.to_string()));
        Ok(json!({
        "ok": true,
        "machineId": machine_id,
            "vmId": vm_cache_key,
            "containerId": container_id,
            "runtime": "docker"
        }))
    }

    pub(crate) fn terminate_vm(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        let entity_id = packet["entityId"]
            .as_str()
            .or_else(|| packet["imageName"].as_str())
            .unwrap_or("main")
            .to_string();
        let container_name = packet["containerName"]
            .as_str()
            .unwrap_or("main")
            .to_string();
        let vm_id = packet["vmId"].as_str().unwrap_or("").to_string();
        let vm_cache_key = if vm_id.is_empty() {
            "main".to_string()
        } else {
            vm_id.clone()
        };
        let container_id = docker_container_id(machine_id, &entity_id, &container_name, &vm_id);
        self.stop_and_remove_if_exists(&container_id)?;
        let mut vm_ctx = GLOBAL_VM_CONTEXT.lock().unwrap();
        vm_ctx.remove(vm_cache_key.as_str());
        Ok(json!({
            "ok": true,
            "machineId": machine_id,
            "containerId": container_id,
            "runtime": "docker"
        }))
    }

    pub(crate) fn exec_vm(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        let (entity_id, container_name, _standalone, vm_id) = extract_docker_identity(packet);
        let command = packet["command"].as_str().unwrap_or("");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        if command.is_empty() {
            return Err("command is required".to_string());
        }
        let container_id = docker_container_id(machine_id, &entity_id, &container_name, &vm_id);
        let create_res = self.with_async(self.docker.create_exec(
            &container_id,
            CreateExecOptions {
                attach_stdout: Some(true),
                attach_stderr: Some(true),
                cmd: Some(vec![
                    "sh".to_string(),
                    "-lc".to_string(),
                    command.to_string(),
                ]),
                ..Default::default()
            },
        ))?;
        let mut output = String::new();
        let start_res = self.with_async(self.docker.start_exec(&create_res.id, None))?;
        if let StartExecResults::Attached {
            output: mut stream, ..
        } = start_res
        {
            tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
                .map_err(|e| format!("tokio runtime init failed: {}", e))?
                .block_on(async {
                    while let Some(msg) = stream.try_next().await? {
                        match msg {
                            LogOutput::StdOut { message }
                            | LogOutput::StdErr { message }
                            | LogOutput::Console { message }
                            | LogOutput::StdIn { message } => {
                                output.push_str(&String::from_utf8_lossy(&message));
                            }
                        }
                    }
                    Ok::<(), BollardError>(())
                })
                .map_err(|e| format!("docker exec stream error: {}", e))?;
        }
        Ok(json!({
            "ok": true,
            "machineId": machine_id,
            "containerId": container_id,
            "output": output
        }))
    }

    pub(crate) fn copy_to_vm(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        let (entity_id, container_name, _standalone, vm_id) = extract_docker_identity(packet);
        let file_name = packet["fileName"].as_str().unwrap_or("");
        let content = packet["content"].as_str().unwrap_or("");
        let target_path = packet["targetPath"].as_str().unwrap_or("/app/input");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        if file_name.is_empty() {
            return Err("fileName is required".to_string());
        }
        let container_id = docker_container_id(machine_id, &entity_id, &container_name, &vm_id);
        let mut files = HashMap::new();
        files.insert(file_name.to_string(), content.as_bytes().to_vec());
        self.upload_files(&container_id, target_path, &files)?;
        Ok(json!({
            "ok": true,
            "machineId": machine_id,
            "containerId": container_id,
            "fileName": file_name
        }))
    }

    fn stop_and_remove_if_exists(&self, container_id: &str) -> Result<(), String> {
        let _ = self.with_async(
            self.docker
                .stop_container(container_id, Some(StopContainerOptions { t: 1 })),
        );
        self.with_async(self.docker.remove_container(
            container_id,
            Some(RemoveContainerOptions {
                force: true,
                v: true,
                ..Default::default()
            }),
        ))
        .or_else(|e| {
            if e.contains("No such container") {
                Ok(())
            } else {
                Err(e)
            }
        })
    }

    fn upload_files(
        &self,
        container_id: &str,
        target_path: &str,
        files: &HashMap<String, Vec<u8>>,
    ) -> Result<(), String> {
        let tar_bytes = build_tar(files)?;
        self.with_async(self.docker.upload_to_container(
            container_id,
            Some(UploadToContainerOptions {
                path: target_path,
                ..Default::default()
            }),
            tar_bytes.into(),
        ))
    }

    pub(crate) fn build_image(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        let build_type = packet["buildType"]
            .as_str()
            .or_else(|| packet["runtime"].as_str())
            .unwrap_or("docker")
            .to_lowercase();
        let entity_id = packet["entityId"]
            .as_str()
            .or_else(|| packet["imageName"].as_str())
            .unwrap_or("main")
            .to_string();
        let image_build_path = packet["imageBuildPath"]
            .as_str()
            .or_else(|| packet["dockerfilePath"].as_str())
            .or_else(|| packet["path"].as_str())
            .unwrap_or("");
        if image_build_path.is_empty() {
            return Err("build path is required".to_string());
        }

        if build_type == "wasm" {
            let script_path = resolve_script_path(image_build_path, "build.sh")?;
            run_local_build_script(&script_path)?;
            return Ok(json!({
                "ok": true,
                "machineId": machine_id,
                "scriptPath": script_path,
                "runtime": "wasm"
            }));
        }

        if build_type == "elpify" {
            let source_path = resolve_script_path(image_build_path, "module.elpify.js")?;
            let source = std::fs::read_to_string(&source_path)
                .map_err(|e| format!("failed to read elpify source {}: {}", source_path, e))?;
            let masm = transpile_js_to_masm(&source)
                .map_err(|e| format!("failed to transpile elpify source: {}", e))?;
            let masm_path = if source_path.ends_with(".elpify.js") {
                source_path.replace(".elpify.js", ".masm")
            } else {
                let source_ref = Path::new(&source_path);
                source_ref
                    .parent()
                    .unwrap_or_else(|| Path::new("."))
                    .join("module.masm")
                    .to_string_lossy()
                    .to_string()
            };
            std::fs::write(&masm_path, masm.as_bytes())
                .map_err(|e| format!("failed to write MASM output {}: {}", masm_path, e))?;
            return Ok(json!({
                "ok": true,
                "machineId": machine_id,
                "sourcePath": source_path,
                "masmPath": masm_path,
                "runtime": "elpify"
            }));
        }

        let image_ref = packet["imageRef"]
            .as_str()
            .map(|s| s.to_string())
            .unwrap_or_else(|| docker_image_ref(machine_id, &entity_id));
        let context = build_context_from_path(image_build_path)?;
        let options = BuildImageOptions {
            dockerfile: "Dockerfile".to_string(),
            t: image_ref.clone(),
            rm: true,
            pull: false,
            ..Default::default()
        };

        tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .map_err(|e| format!("tokio runtime init failed: {}", e))?
            .block_on(async {
                let mut stream = self.docker.build_image(options, None, Some(context.into()));
                while let Some(update) = stream.try_next().await.map_err(|e| e.to_string())? {
                    if let Some(status) = update.stream.clone() {
                        emit_vm_log("main", "build", status.trim());
                    }
                    if let Some(error) = update.error {
                        emit_vm_log("main", "build", error.trim());
                        return Err(format!("docker build failed: {}", error));
                    }
                }
                Ok::<(), String>(())
            })?;

        Ok(json!({
            "ok": true,
            "machineId": machine_id,
            "imageRef": image_ref,
            "runtime": "docker"
        }))
    }
}

fn docker_container_id(
    machine_id: &str,
    entity_id: &str,
    container_name: &str,
    vm_id: &str,
) -> String {
    if !vm_id.is_empty() {
        return format!("{}_{}", machine_id.replace('@', "_"), vm_id);
    }
    format!(
        "{}_{}_{}",
        machine_id.replace('@', "_"),
        entity_id,
        container_name
    )
}

fn docker_image_ref(machine_id: &str, entity_id: &str) -> String {
    format!("{}/{}", machine_id.replace('@', "_"), entity_id)
}

fn extract_docker_identity(packet: &JsonValue) -> (String, String, bool, String) {
    let standalone = packet["standalone"].as_bool().unwrap_or(false)
        || packet["isStandalone"].as_bool().unwrap_or(false);
    let entity_id = packet["entityId"]
        .as_str()
        .or_else(|| packet["imageName"].as_str())
        .unwrap_or("main")
        .to_string();
    let container_name = packet["containerName"]
        .as_str()
        .unwrap_or("main")
        .to_string();
    let vm_id = packet["vmId"].as_str().unwrap_or("").to_string();
    (entity_id, container_name, standalone, vm_id)
}

fn build_tar(files: &HashMap<String, Vec<u8>>) -> Result<Vec<u8>, String> {
    let mut buf = Vec::<u8>::new();
    {
        let mut tar = TarBuilder::new(&mut buf);
        for (name, content) in files {
            let mut header = tar::Header::new_gnu();
            header.set_path(name).map_err(|e| e.to_string())?;
            header.set_size(content.len() as u64);
            header.set_mode(0o644);
            header.set_cksum();
            tar.append(&header, Cursor::new(content.as_slice()))
                .map_err(|e| e.to_string())?;
        }
        tar.finish().map_err(|e| e.to_string())?;
    }
    Ok(buf)
}

fn parse_input_files(input_files: &JsonValue) -> Result<HashMap<String, Vec<u8>>, String> {
    if let Some(serialized) = input_files.as_str() {
        let parsed = serde_json::from_str::<JsonValue>(serialized)
            .map_err(|e| format!("inputFiles must be valid JSON: {}", e))?;
        return parse_input_files(&parsed);
    }
    let mut files = HashMap::new();
    let obj = input_files
        .as_object()
        .ok_or_else(|| "inputFiles must be an object or JSON-encoded object".to_string())?;
    for (key, value) in obj {
        let raw = value.as_str().unwrap_or("");
        files.insert(key.to_string(), raw.as_bytes().to_vec());
    }
    Ok(files)
}

fn emit_vm_log(vm_id: &str, log_type: &str, text: &str) {
    if vm_id.trim().is_empty() || text.trim().is_empty() {
        return;
    }
    let packet = json!({
        "key": "vmLog",
        "input": {
            "vmId": vm_id,
            "logType": log_type,
            "text": text
        }
    });
    let _ = wasm_send(packet);
}

fn build_context_from_path(path: &str) -> Result<Vec<u8>, String> {
    let path_ref = Path::new(path);
    if !path_ref.exists() {
        return Err(format!("dockerfile path does not exist: {}", path));
    }
    let mut buf = Vec::<u8>::new();
    {
        let mut tar = TarBuilder::new(&mut buf);
        if path_ref.is_dir() {
            tar.append_dir_all(".", path_ref)
                .map_err(|e| format!("failed to archive docker context directory: {}", e))?;
        } else {
            let parent = path_ref.parent().unwrap_or_else(|| Path::new("."));
            tar.append_dir_all(".", parent)
                .map_err(|e| format!("failed to archive docker context parent directory: {}", e))?;
        }
        tar.finish().map_err(|e| e.to_string())?;
    }
    Ok(buf)
}

fn resolve_script_path(path: &str, default_script_name: &str) -> Result<String, String> {
    let path_ref = Path::new(path);
    if !path_ref.exists() {
        return Err(format!("build path does not exist: {}", path));
    }
    if path_ref.is_file() {
        return Ok(path.to_string());
    }
    let script_path = path_ref.join(default_script_name);
    if !script_path.exists() {
        return Err(format!(
            "required build script not found at {}",
            script_path.to_string_lossy()
        ));
    }
    Ok(script_path.to_string_lossy().to_string())
}

fn run_local_build_script(script_path: &str) -> Result<(), String> {
    let script_ref = Path::new(script_path);
    let script_name = script_ref
        .file_name()
        .ok_or_else(|| format!("invalid build script path: {}", script_path))?
        .to_string_lossy()
        .to_string();
    let cwd = script_ref
        .parent()
        .unwrap_or_else(|| Path::new("."))
        .to_path_buf();
    let output = Command::new("bash")
        .arg(script_name)
        .current_dir(cwd)
        .output()
        .map_err(|e| format!("failed to execute wasm build script {}: {}", script_path, e))?;
    if output.status.success() {
        return Ok(());
    }
    Err(format!(
        "wasm build script failed (status={}): {}{}",
        output.status,
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    ))
}

impl VmController for DockerVmController {
    fn build_image(packet: &JsonValue) -> Result<JsonValue, String> {
        let controller = Self::new()?;
        controller.build_image(packet)
    }

    fn create(packet: &JsonValue) -> Result<JsonValue, String> {
        let controller = Self::new()?;
        controller.run_vm(packet)
    }

    fn starts(packet: &JsonValue) -> Result<JsonValue, String> {
        let controller = Self::new()?;
        controller.run_vm(packet)
    }

    fn stop(packet: &JsonValue) -> Result<JsonValue, String> {
        let controller = Self::new()?;
        controller.terminate_vm(packet)
    }

    fn resume(packet: &JsonValue) -> Result<JsonValue, String> {
        let controller = Self::new()?;
        controller.run_vm(packet)
    }

    fn pause(packet: &JsonValue) -> Result<JsonValue, String> {
        let controller = Self::new()?;
        controller.terminate_vm(packet)
    }

    fn exec(packet: &JsonValue) -> Result<JsonValue, String> {
        let controller = Self::new()?;
        controller.exec_vm(packet)
    }

    fn copy_to(packet: &JsonValue) -> Result<JsonValue, String> {
        let controller = Self::new()?;
        controller.copy_to_vm(packet)
    }

    fn copy_from(packet: &JsonValue) -> Result<JsonValue, String> {
        let _ = packet;
        Err("copy_from is not implemented yet for docker runtime".to_string())
    }
}
