use crate::prelude::*;
use crate::globals::{GLOBAL_VM_CONTEXT, GLOBAL_DB};
use crate::controllers::docker_vm_controller::DockerVmController;
use crate::controllers::fire_vm_controller::FireVmController;
use crate::models::vm_runtime::{verify_program_execution_from_packet, parse_u64_array_field, parse_u8_array_field};
use crate::bridge::messaging::wasm_send;
use crate::host::functions::*;

pub(crate) struct HostHierarchy {
    pub(crate) creature_id: String,
    pub(crate) program_id: String,
    pub(crate) entity_name: String,
    pub(crate) entity_path: String,
}

#[derive(Default)]
pub(crate) struct CachedVmHierarchy {
    pub(crate) creature_id: String,
    pub(crate) program_id: String,
}

fn resolve_cached_vm_hierarchy(input: &JsonValue) -> CachedVmHierarchy {
    let vm_id = input["vmId"].as_str().unwrap_or("").trim();
    if vm_id.is_empty() {
        return CachedVmHierarchy::default();
    }

    let vm_ctx = GLOBAL_VM_CONTEXT.lock().unwrap();
    if let Some((creature_id, program_id)) = vm_ctx.get(vm_id) {
        return CachedVmHierarchy {
            creature_id: creature_id.clone(),
            program_id: program_id.clone(),
        };
    }
    CachedVmHierarchy::default()
}

fn value_from_packet_or_input<'a>(
    packet: &'a JsonValue,
    input: &'a JsonValue,
    key: &str,
) -> &'a str {
    packet[key]
        .as_str()
        .filter(|v| !v.is_empty())
        .or_else(|| input[key].as_str().filter(|v| !v.is_empty()))
        .unwrap_or("")
}

pub(crate) fn resolve_host_hierarchy(packet: &JsonValue, input: &JsonValue) -> HostHierarchy {
    let cached = resolve_cached_vm_hierarchy(input);

    let creature_from_req = value_from_packet_or_input(packet, input, "creatureId");
    let creature_id_owned = if !cached.creature_id.is_empty() {
        cached.creature_id
    } else {
        creature_from_req.to_string()
    };

    let program_from_req = value_from_packet_or_input(packet, input, "programId");
    let program_id_owned = if !cached.program_id.is_empty() {
        cached.program_id
    } else {
        program_from_req.to_string()
    };

    let entity_name = value_from_packet_or_input(packet, input, "entityName").to_string();
    let entity_path = packet["entityPath"]
        .as_str()
        .filter(|v| !v.is_empty())
        .or_else(|| packet["astPath"].as_str().filter(|v| !v.is_empty()))
        .or_else(|| input["entityPath"].as_str().filter(|v| !v.is_empty()))
        .or_else(|| input["astPath"].as_str().filter(|v| !v.is_empty()))
        .or_else(|| input["astpath"].as_str().filter(|v| !v.is_empty()))
        .unwrap_or("")
        .to_string();
    HostHierarchy {
        creature_id: creature_id_owned,
        program_id: program_id_owned,
        entity_name,
        entity_path,
    }
}

pub(crate) fn run_db_op(ctx: &HostHierarchy, input: &JsonValue) -> Result<String, String> {
    let op = input["op"].as_str().unwrap_or("");
    let key = input["key"].as_str().unwrap_or("");
    let db_prefix = if !ctx.creature_id.is_empty() && !ctx.program_id.is_empty() {
        if !ctx.entity_name.is_empty() && !ctx.entity_path.is_empty() {
            format!(
                "{}::{}::{}::{}",
                ctx.creature_id, ctx.program_id, ctx.entity_name, ctx.entity_path
            )
        } else if !ctx.entity_name.is_empty() {
            format!(
                "{}::{}::{}",
                ctx.creature_id, ctx.program_id, ctx.entity_name
            )
        } else {
            format!("{}::{}", ctx.creature_id, ctx.program_id)
        }
    } else if !ctx.program_id.is_empty() {
        ctx.program_id.to_string()
    } else {
        ctx.creature_id.to_string()
    };
    let namespaced_key = format!("{}::{}", db_prefix, key);
    let db = GLOBAL_DB.lock().unwrap();
    match op {
        "put" => {
            let val = input["val"].as_str().unwrap_or("");
            db.put(namespaced_key.as_bytes(), val.as_bytes())
                .map_err(|e| format!("db put failed: {}", e))?;
            Ok("{}".to_string())
        }
        "get" => {
            let val = db
                .get(namespaced_key.as_bytes())
                .map_err(|e| format!("db get failed: {}", e))?;
            let val_str = val
                .as_ref()
                .and_then(|v| str::from_utf8(v).ok())
                .unwrap_or("")
                .to_string();
            Ok(json!({"data": val_str}).to_string())
        }
        "del" => {
            db.delete(namespaced_key.as_bytes())
                .map_err(|e| format!("db delete failed: {}", e))?;
            Ok("{}".to_string())
        }
        "getByPrefix" => {
            let prefix = input["prefix"].as_str().unwrap_or("");
            let namespaced_prefix = format!("{}::{}", db_prefix, prefix);
            let mut vals = Vec::<String>::new();
            for item in db.prefix_iterator(namespaced_prefix.as_bytes()) {
                let (_, val) = item.map_err(|e| format!("db prefix iteration failed: {}", e))?;
                vals.push(String::from_utf8_lossy(&val).to_string());
            }
            Ok(json!({"data": vals}).to_string())
        }
        _ => Err("unsupported db op".to_string()),
    }
}

pub(crate) fn perform_http_request(input: &JsonValue) -> Result<String, String> {
    let mut url = input["url"].as_str().unwrap_or("").to_string();
    if url.is_empty() {
        return Err("url is required".to_string());
    }

    let mut method = input["method"].as_str().unwrap_or("").trim().to_uppercase();
    if method.is_empty() {
        if let Some((prefixed_method, rest_url)) = url.split_once('|') {
            method = prefixed_method.trim().to_uppercase();
            url = rest_url.to_string();
        } else {
            method = "POST".to_string();
        }
    }

    let http_method =
        Method::from_bytes(method.as_bytes()).map_err(|e| format!("invalid http method: {}", e))?;

    let mut request = Client::new().request(http_method, url);

    match &input["headers"] {
        JsonValue::Object(headers_obj) => {
            for (k, v) in headers_obj {
                if let Some(value) = v.as_str() {
                    request = request.header(k, value);
                } else {
                    request = request.header(k, v.to_string());
                }
            }
        }
        JsonValue::String(headers_raw) => {
            if !headers_raw.trim().is_empty() {
                let parsed_headers: JsonValue = serde_json::from_str(headers_raw)
                    .map_err(|e| format!("invalid headers json: {}", e))?;
                if let Some(headers_obj) = parsed_headers.as_object() {
                    for (k, v) in headers_obj {
                        if let Some(value) = v.as_str() {
                            request = request.header(k, value);
                        } else {
                            request = request.header(k, v.to_string());
                        }
                    }
                } else {
                    return Err("headers must be a JSON object".to_string());
                }
            }
        }
        JsonValue::Null => {}
        _ => return Err("headers must be a JSON object or stringified JSON object".to_string()),
    }

    if let Some(body) = input["body"].as_str() {
        request = request.body(body.to_string());
    } else if !input["body"].is_null() {
        request = request.body(input["body"].to_string());
    }

    let response = request
        .send()
        .map_err(|e| format!("http request failed: {}", e))?;
    let bytes = response
        .bytes()
        .map_err(|e| format!("failed to read response body: {}", e))?;
    Ok(BASE64_STANDARD.encode(bytes))
}

pub(crate) fn handle_unified_host_call(packet: &JsonValue) -> String {
    let op = packet["op"]
        .as_str()
        .or_else(|| packet["key"].as_str())
        .unwrap_or("");
    let mut input = if packet["input"].is_null() {
        JsonValue::Null
    } else {
        packet["input"].clone()
    };
    let ctx = resolve_host_hierarchy(packet, &input);
    if ctx.program_id.is_empty() {
        return json!({"ok": false, "error": "programId is required"}).to_string();
    }
    if let Some(input_obj) = input.as_object_mut() {
        if input_obj
            .get("programId")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .is_empty()
        {
            input_obj.insert(
                "programId".to_string(),
                JsonValue::String(ctx.program_id.clone()),
            );
        }
        if input_obj
            .get("machineId")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .is_empty()
        {
            input_obj.insert(
                "machineId".to_string(),
                JsonValue::String(ctx.program_id.clone()),
            );
        }
    }
    match op {
        "dbOp" => host_fn_db_op(&ctx, &input),
        "runVm" => host_fn_run_vm(&input),
        "terminateVm" => host_fn_terminate_vm(&input),
        "execVm" | "execDocker" => host_fn_exec_vm(&input),
        "copyToVm" | "copyToDocker" => host_fn_copy_to_vm(&input),
        "buildVmImage" | "buildDockerImage" => host_fn_build_vm_image(&input),
        "httpPost" | "httpRequest" => host_fn_http_request(&input),
        "elpifyProof" | "verifyProgramExecution" => host_fn_verify_program(&input),
        "protocolApi" | "callProtocolApi" => host_fn_protocol_api(&input),
        "signal" => host_fn_signal(&input),
        "createAccess" | "createOwnedAccess" => host_fn_create_access(&input),
        "deleteAccess" | "removeAccess" | "deleteOwnedAccess" | "removeOwnedAccess" => {
            host_fn_delete_access(&input)
        }
        "createStore" | "createOwnedStore" => host_fn_create_store(&input),
        "deleteStore" | "removeStore" | "deleteOwnedStore" | "removeOwnedStore" => {
            host_fn_delete_store(&input)
        }
        "createCreature" | "createOwnedCreature" => host_fn_create_creature(&input),
        "validateSign" => host_fn_validate_sign(&input),
        "transfer" => host_fn_transfer(&input),
        "consumeLock" => host_fn_consume_lock(&input),
        "lockToken" => host_fn_lock_token(&input),
        "createProgram" => host_fn_create_program(&input),
        "deleteProgram" | "deleteOwnedProgram" => host_fn_delete_program(&input),
        "deployEntity" | "deploy entity" => host_fn_deploy_entity(&input),
        "deleteCreature" | "removeCreature" | "deleteOwnedCreature" | "removeOwnedCreature" => {
            host_fn_delete_creature(&input)
        }
        _ => {
            let packet = json!({
                "key": op,
                "input": input
            });
            wasm_send(packet)
        }
    }
}

pub(crate) fn with_docker_controller<T, F>(f: F) -> Result<T, String>
where
    F: FnOnce(&DockerVmController) -> Result<T, String>,
{
    let controller = DockerVmController::new()?;
    f(&controller)
}

pub(crate) fn with_fire_controller<T, F>(f: F) -> Result<T, String>
where
    F: FnOnce(&FireVmController) -> Result<T, String>,
{
    let controller = FireVmController::new()?;
    f(&controller)
}
