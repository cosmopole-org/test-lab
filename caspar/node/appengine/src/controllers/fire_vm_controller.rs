use crate::prelude::*;
use crate::controllers::vm_controller::VmController;
use crate::bridge::messaging::{wasm_send, log};
use crate::models::vm_runtime::{parse_vm_resource_limits, VmResourceLimits};
use crate::network::vm_network::VmNetworkService;
use crate::globals::GLOBAL_VM_CONTEXT;

pub(crate) static GLOBAL_FIRE_VMS: Lazy<Arc<Mutex<HashMap<String, FireVmProcess>>>> =
    Lazy::new(|| Arc::new(Mutex::new(HashMap::new())));

pub(crate) struct FireVmProcess {
    pub(crate) machine_id: String,
    pub(crate) vm_id: String,
    pub(crate) requester_user_id: String,
    pub(crate) stream_store_id: String,
    pub(crate) socket_path: PathBuf,
    pub(crate) child: Child,
    pub(crate) stdin: Arc<Mutex<ChildStdin>>,
    pub(crate) output: Arc<Mutex<String>>,
    pub(crate) io_stop: Arc<AtomicBool>,
    pub(crate) stdout_thread: Option<JoinHandle<()>>,
    pub(crate) stderr_thread: Option<JoinHandle<()>>,
}

pub(crate) struct FireVmController;

impl FireVmController {
    pub(crate) fn new() -> Result<Self, String> {
        std::fs::create_dir_all("/opt/firecracker/vms")
            .map_err(|e| format!("failed to prepare firecracker vm dir: {}", e))?;
        Ok(Self)
    }

    pub(crate) fn run_vm(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("").trim();
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        let vm_id = packet["vmId"].as_str().unwrap_or("main").trim();
        let vm_cache_key = if vm_id.is_empty() { "main" } else { vm_id };
        let requester_user_id = packet["requesterUserId"]
            .as_str()
            .unwrap_or("")
            .trim()
            .to_string();
        let requester_user_id_for_cache = requester_user_id.clone();
        let stream_store_id = packet["storeId"].as_str().unwrap_or("").trim().to_string();
        let process_key = fire_process_key(machine_id, vm_id);
        let socket_path = fire_socket_path(machine_id, vm_id);
        let limits = parse_vm_resource_limits(packet);

        self.terminate_by_key(&process_key);
        let _ = std::fs::remove_file(&socket_path);

        let child = Command::new("/usr/local/bin/firecracker")
            .arg("--api-sock")
            .arg(&socket_path)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .map_err(|e| format!("failed to start firecracker process: {}", e))?;
        configure_firecracker_machine_limits(&socket_path, &limits)?;

        let mut child = child;
        let stdin = child
            .stdin
            .take()
            .ok_or_else(|| "failed to acquire firecracker stdin".to_string())?;
        let stdout = child
            .stdout
            .take()
            .ok_or_else(|| "failed to acquire firecracker stdout".to_string())?;
        let stderr = child
            .stderr
            .take()
            .ok_or_else(|| "failed to acquire firecracker stderr".to_string())?;

        let output = Arc::new(Mutex::new(String::new()));
        let io_stop = Arc::new(AtomicBool::new(false));

        let output_stdout = Arc::clone(&output);
        let io_stop_stdout = Arc::clone(&io_stop);
        let machine_id_stdout = machine_id.to_string();
        let vm_id_stdout = vm_id.to_string();
        let requester_stdout = requester_user_id.clone();
        let store_id_stdout = stream_store_id.clone();
        let stdout_thread = thread::spawn(move || {
            let mut reader = std::io::BufReader::new(stdout);
            let mut line = String::new();
            while !io_stop_stdout.load(Ordering::Relaxed) {
                line.clear();
                match std::io::BufRead::read_line(&mut reader, &mut line) {
                    Ok(0) => break,
                    Ok(_) => {
                        if let Ok(mut out) = output_stdout.lock() {
                            out.push_str(&line);
                        }
                        emit_fire_output_signal(
                            &machine_id_stdout,
                            &vm_id_stdout,
                            &requester_stdout,
                            &store_id_stdout,
                            line.trim_end(),
                        );
                    }
                    Err(_) => break,
                }
            }
        });

        let output_stderr = Arc::clone(&output);
        let io_stop_stderr = Arc::clone(&io_stop);
        let machine_id_stderr = machine_id.to_string();
        let vm_id_stderr = vm_id.to_string();
        let requester_stderr = requester_user_id.clone();
        let store_id_stderr = stream_store_id.clone();
        let stderr_thread = thread::spawn(move || {
            let mut reader = std::io::BufReader::new(stderr);
            let mut line = String::new();
            while !io_stop_stderr.load(Ordering::Relaxed) {
                line.clear();
                match std::io::BufRead::read_line(&mut reader, &mut line) {
                    Ok(0) => break,
                    Ok(_) => {
                        if let Ok(mut out) = output_stderr.lock() {
                            out.push_str(&line);
                        }
                        emit_fire_output_signal(
                            &machine_id_stderr,
                            &vm_id_stderr,
                            &requester_stderr,
                            &store_id_stderr,
                            line.trim_end(),
                        );
                    }
                    Err(_) => break,
                }
            }
        });

        let mut fire_vms = GLOBAL_FIRE_VMS.lock().unwrap();
        fire_vms.insert(
            process_key.clone(),
            FireVmProcess {
                machine_id: machine_id.to_string(),
                vm_id: vm_id.to_string(),
                requester_user_id,
                stream_store_id,
                socket_path,
                child,
                stdin: Arc::new(Mutex::new(stdin)),
                output,
                io_stop,
                stdout_thread: Some(stdout_thread),
                stderr_thread: Some(stderr_thread),
            },
        );
        let mut vm_ctx = GLOBAL_VM_CONTEXT.lock().unwrap();
        vm_ctx.insert(
            vm_cache_key.to_string(),
            (requester_user_id_for_cache, machine_id.to_string()),
        );
        let timeout_key = process_key.clone();
        let timeout_secs = limits.max_exec_time_secs;
        thread::spawn(move || {
            thread::sleep(Duration::from_secs(timeout_secs));
            let mut fire_vms = GLOBAL_FIRE_VMS.lock().unwrap();
            if let Some(mut proc) = fire_vms.remove(&timeout_key) {
                proc.io_stop.store(true, Ordering::Relaxed);
                let _ = proc.child.kill();
                let _ = proc.child.wait();
                if let Some(handle) = proc.stdout_thread.take() {
                    let _ = handle.join();
                }
                if let Some(handle) = proc.stderr_thread.take() {
                    let _ = handle.join();
                }
                let _ = std::fs::remove_file(proc.socket_path);
                let mut vm_ctx = GLOBAL_VM_CONTEXT.lock().unwrap();
                vm_ctx.remove(proc.vm_id.as_str());
                let _ = wasm_send(json!({
                    "key": "vmLog",
                    "input": {
                        "vmId": proc.vm_id,
                        "logType": "runtime",
                        "text": format!("fire vm terminated due to max execution time ({}s)", timeout_secs),
                    }
                }));
            }
        });

        Ok(json!({
            "ok": true,
            "runtime": "fire",
            "machineId": machine_id,
            "vmId": vm_id,
            "processKey": process_key,
            "resources": {
                "maxExecTimeSeconds": limits.max_exec_time_secs,
                "ramMb": limits.ram_mb,
                "diskGb": limits.disk_gb,
                "cpuCores": limits.cpu_cores
            }
        }))
    }

    pub(crate) fn terminate_vm(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("").trim();
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        let vm_id = packet["vmId"].as_str().unwrap_or("main").trim();
        let vm_cache_key = if vm_id.is_empty() { "main" } else { vm_id };
        let process_key = fire_process_key(machine_id, vm_id);
        self.terminate_by_key(&process_key);
        let mut vm_ctx = GLOBAL_VM_CONTEXT.lock().unwrap();
        vm_ctx.remove(vm_cache_key);

        Ok(json!({
            "ok": true,
            "runtime": "fire",
            "machineId": machine_id,
            "vmId": vm_id,
            "processKey": process_key,
        }))
    }

    pub(crate) fn exec_vm(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("").trim();
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        let vm_id = packet["vmId"].as_str().unwrap_or("main").trim();
        let command = packet["command"].as_str().unwrap_or("").trim();
        if command.is_empty() {
            return Err("command is required".to_string());
        }

        let process_key = fire_process_key(machine_id, vm_id);
        let (stdin_arc, output_arc) = {
            let fire_vms = GLOBAL_FIRE_VMS.lock().unwrap();
            let proc = fire_vms
                .get(&process_key)
                .ok_or_else(|| format!("fire vm is not running: {}", process_key))?;
            (Arc::clone(&proc.stdin), Arc::clone(&proc.output))
        };

        {
            let mut stdin = stdin_arc
                .lock()
                .map_err(|_| "failed to lock fire vm stdin".to_string())?;
            std::io::Write::write_all(&mut *stdin, command.as_bytes())
                .map_err(|e| format!("failed to write command to fire vm: {}", e))?;
            std::io::Write::write_all(&mut *stdin, b"\n")
                .map_err(|e| format!("failed to write command newline to fire vm: {}", e))?;
            std::io::Write::flush(&mut *stdin)
                .map_err(|e| format!("failed to flush fire vm stdin: {}", e))?;
        }

        thread::sleep(Duration::from_millis(100));
        let output = output_arc
            .lock()
            .map_err(|_| "failed to lock fire vm output buffer".to_string())?
            .clone();

        Ok(json!({
            "ok": true,
            "runtime": "fire",
            "machineId": machine_id,
            "vmId": vm_id,
            "output": output,
        }))
    }

    pub(crate) fn copy_to_vm(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("").trim();
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        let file_name = packet["fileName"].as_str().unwrap_or("").trim();
        if file_name.is_empty() {
            return Err("fileName is required".to_string());
        }
        let target_path = packet["targetPath"].as_str().unwrap_or("/tmp").trim();
        let content = packet["content"].as_str().unwrap_or("");
        let escaped_content = content.replace('\\', "\\\\").replace('\'', "'\"'\"'");
        let copy_cmd = format!(
            "mkdir -p '{}' && printf '%s' '{}' > '{}/{}'",
            target_path, escaped_content, target_path, file_name
        );
        let mut copy_packet = packet.clone();
        copy_packet["command"] = JsonValue::String(copy_cmd);
        copy_packet["vmId"] =
            JsonValue::String(packet["vmId"].as_str().unwrap_or("main").to_string());
        self.exec_vm(&copy_packet)?;
        Ok(json!({
            "ok": true,
            "runtime": "fire",
            "machineId": machine_id,
            "fileName": file_name,
        }))
    }

    pub(crate) fn build_image(&self, packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("").trim();
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        let vm_id = packet["vmId"].as_str().unwrap_or("main");
        let _ = wasm_send(json!({
            "key": "vmLog",
            "input": {
                "vmId": vm_id,
                "logType": "build",
                "text": "fire vm build image request accepted",
            }
        }));
        Ok(json!({
            "ok": true,
            "runtime": "fire",
            "machineId": machine_id,
        }))
    }

    fn terminate_by_key(&self, process_key: &str) {
        let mut fire_vms = GLOBAL_FIRE_VMS.lock().unwrap();
        if let Some(mut proc) = fire_vms.remove(process_key) {
            proc.io_stop.store(true, Ordering::Relaxed);
            let _ = proc.child.kill();
            let _ = proc.child.wait();
            if let Some(handle) = proc.stdout_thread.take() {
                let _ = handle.join();
            }
            if let Some(handle) = proc.stderr_thread.take() {
                let _ = handle.join();
            }
            let _ = std::fs::remove_file(proc.socket_path);
            let _machine_id = proc.machine_id;
            let vm_id = proc.vm_id;
            let _requester_user_id = proc.requester_user_id;
            let _stream_store_id = proc.stream_store_id;
            let mut vm_ctx = GLOBAL_VM_CONTEXT.lock().unwrap();
            vm_ctx.remove(vm_id.as_str());
        }
    }
}

fn configure_firecracker_machine_limits(
    socket_path: &PathBuf,
    limits: &VmResourceLimits,
) -> Result<(), String> {
    for _ in 0..50 {
        if socket_path.exists() {
            break;
        }
        thread::sleep(Duration::from_millis(50));
    }
    if !socket_path.exists() {
        return Err(format!(
            "firecracker socket did not become available: {}",
            socket_path.display()
        ));
    }

    let mut stream = std::os::unix::net::UnixStream::connect(socket_path)
        .map_err(|e| format!("failed to connect firecracker socket: {}", e))?;
    let body = json!({
        "vcpu_count": limits.cpu_cores,
        "mem_size_mib": limits.ram_mb,
        "track_dirty_pages": false
    })
    .to_string();
    let req = format!(
        "PUT /machine-config HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
        body.len(),
        body
    );
    stream
        .write_all(req.as_bytes())
        .map_err(|e| format!("failed to write firecracker machine-config: {}", e))?;
    stream
        .flush()
        .map_err(|e| format!("failed to flush firecracker machine-config: {}", e))?;

    let mut response = String::new();
    let _ = stream.read_to_string(&mut response);
    if response.starts_with("HTTP/1.1 204") || response.starts_with("HTTP/1.1 200") {
        Ok(())
    } else {
        Err(format!(
            "firecracker machine-config rejected response={}",
            response.lines().next().unwrap_or("")
        ))
    }
}

fn fire_process_key(machine_id: &str, vm_id: &str) -> String {
    format!("{}_{}", machine_id.replace('@', "_"), vm_id)
}

fn fire_socket_path(machine_id: &str, vm_id: &str) -> PathBuf {
    PathBuf::from(VmNetworkService::firecracker_socket(
        &machine_id.replace('@', "_"),
        vm_id,
    ))
}

fn emit_fire_output_signal(
    creature_id: &str,
    vm_id: &str,
    requester_user_id: &str,
    store_id: &str,
    output_line: &str,
) {
    if requester_user_id.is_empty() || store_id.is_empty() || output_line.is_empty() {
        return;
    }
    let payload = json!({
        "event": "fireVmOutput",
        "creatureId": creature_id,
        "vmId": vm_id,
        "requesterUserId": requester_user_id,
        "output": output_line,
    })
    .to_string();
    let signal_packet = json!({
        "key": "signal",
        "input": {
            "machineId": creature_id,
            "creatureId": creature_id,
            "storeId": store_id,
            "userId": requester_user_id,
            "type": "fire.vm.output",
            "temp": true,
            "data": payload,
        }
    });
    let _ = wasm_send(signal_packet);
    let vm_log_packet = json!({
        "key": "vmLog",
        "input": {
            "vmId": vm_id,
            "logType": "runtime",
            "text": output_line,
        }
    });
    let _ = wasm_send(vm_log_packet);
}

impl VmController for FireVmController {
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
        Err("copy_from is not implemented yet for fire runtime".to_string())
    }
}
