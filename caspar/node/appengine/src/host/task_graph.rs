use crate::prelude::*;
use crate::models::vm_runtime::{WasmMac, HostData, verify_program_execution_from_packet, parse_u64_array_field, parse_u8_array_field, acquire_resource_lock, release_resource_lock};
use crate::bridge::messaging::{log_vm, wasm_send};
use crate::host::vm_host_functions::{with_docker_controller, with_fire_controller, perform_http_request};

pub fn host_call(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };
    let mem = _caller.memory_mut(0).unwrap();

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem
        .get_data(in_offset.cast_unsigned(), in_l.cast_unsigned())
        .unwrap_or_default();
    let req_raw = str::from_utf8(&in_bytes).unwrap_or("{}");
    let req: JsonValue = serde_json::from_str(req_raw).unwrap_or_default();
    let op = req["op"].as_str().unwrap_or("");

    let res_str = match op {
        "output" => {
            rt.execution_result = req["input"]["text"].as_str().unwrap_or("").to_string();
            rt.has_output = true;
            "{}".to_string()
        }
        "consoleLog" => {
            log_vm(
                req["input"]["text"].as_str().unwrap_or("").to_string(),
                rt.vm_id.clone(),
                "runtime",
            );
            "{}".to_string()
        }
        "dbOp" => {
            let op_type = req["input"]["op"].as_str().unwrap_or("");
            if op_type == "put" {
                let key = req["input"]["key"].as_str().unwrap_or("");
                let val = req["input"]["val"].as_str().unwrap_or("");
                rt.trx
                    .put(format!("{}::{}", rt.machine_id, key), val.to_string());
                "{}".to_string()
            } else if op_type == "del" {
                let key = req["input"]["key"].as_str().unwrap_or("");
                rt.trx.del(format!("{}::{}", rt.machine_id, key));
                "{}".to_string()
            } else if op_type == "get" {
                let key = req["input"]["key"].as_str().unwrap_or("");
                rt.trx.get(format!("{}::{}", rt.machine_id, key))
            } else if op_type == "getByPrefix" {
                let prefix = req["input"]["prefix"].as_str().unwrap_or("");
                let vals = rt
                    .trx
                    .get_by_prefix(format!("{}::{}", rt.machine_id, prefix));
                json!({"data": vals}).to_string()
            } else {
                "{}".to_string()
            }
        }
        "lockResource" => {
            let runtime = req["input"]["runtime"].as_str().unwrap_or("wasm");
            if runtime != "wasm"
                && runtime != "docker"
                && runtime != "javascript"
                && runtime != "elpian"
            {
                json!({"ok": false, "error": "lock API is only available for wasm, docker, javascript and elpian runtimes"})
                    .to_string()
            } else {
                let resource_id = req["input"]["resourceId"].as_str().unwrap_or("");
                let owner_id = req["input"]["ownerId"]
                    .as_str()
                    .unwrap_or(rt.machine_id.as_str());
                match acquire_resource_lock(resource_id, owner_id) {
                    Ok(()) => json!({"ok": true}).to_string(),
                    Err(err) => json!({"ok": false, "error": err}).to_string(),
                }
            }
        }
        "unlockResource" => {
            let runtime = req["input"]["runtime"].as_str().unwrap_or("wasm");
            if runtime != "wasm"
                && runtime != "docker"
                && runtime != "javascript"
                && runtime != "elpian"
            {
                json!({"ok": false, "error": "unlock API is only available for wasm, docker, javascript and elpian runtimes"})
                    .to_string()
            } else {
                let resource_id = req["input"]["resourceId"].as_str().unwrap_or("");
                let owner_id = req["input"]["ownerId"]
                    .as_str()
                    .unwrap_or(rt.machine_id.as_str());
                match release_resource_lock(resource_id, owner_id) {
                    Ok(()) => json!({"ok": true}).to_string(),
                    Err(err) => json!({"ok": false, "error": err}).to_string(),
                }
            }
        }
        "runVm" => {
            if req["input"]["runtime"].as_str().unwrap_or("") == "docker" {
                match with_docker_controller(|controller| controller.run_vm(&req["input"])) {
                    Ok(res) => res.to_string(),
                    Err(err) => json!({"ok": false, "error": err}).to_string(),
                }
            } else {
                (rt.callback)(req.clone())
            }
        }
        "terminateVm" => {
            if req["input"]["runtime"].as_str().unwrap_or("") == "docker" {
                match with_docker_controller(|controller| controller.terminate_vm(&req["input"])) {
                    Ok(res) => res.to_string(),
                    Err(err) => json!({"ok": false, "error": err}).to_string(),
                }
            } else {
                (rt.callback)(req.clone())
            }
        }
        "execVm" | "execDocker" => {
            match with_docker_controller(|controller| controller.exec_vm(&req["input"])) {
                Ok(res) => res.to_string(),
                Err(err) => json!({"ok": false, "error": err}).to_string(),
            }
        }
        "copyToVm" | "copyToDocker" => {
            match with_docker_controller(|controller| controller.copy_to_vm(&req["input"])) {
                Ok(res) => res.to_string(),
                Err(err) => json!({"ok": false, "error": err}).to_string(),
            }
        }
        "buildVmImage" | "buildDockerImage" => {
            match with_docker_controller(|controller| controller.build_image(&req["input"])) {
                Ok(res) => res.to_string(),
                Err(err) => json!({"ok": false, "error": err}).to_string(),
            }
        }
        "httpPost" | "httpRequest" => match perform_http_request(&req["input"]) {
            Ok(res) => res,
            Err(err) => json!({"ok": false, "error": err}).to_string(),
        },
        "elpifyProof" | "verifyProgramExecution" => {
            let masm_path = req["input"]["masmPath"].as_str().unwrap_or("").to_string();
            let inputs = parse_u64_array_field(&req["input"], "inputs");
            let outputs = parse_u64_array_field(&req["input"], "outputs");
            let proof_bytes = parse_u8_array_field(&req["input"], "proof");

            match verify_program_execution_from_packet(&masm_path, &inputs, &outputs, &proof_bytes)
            {
                Ok(security) => json!({"ok": true, "security": security}).to_string(),
                Err(err) => json!({"ok": false, "error": err}).to_string(),
            }
        }
        _ => {
            let key = req["key"].as_str().unwrap_or("");
            let packet = if key.is_empty() {
                json!({
                    "key": op,
                    "input": req["input"].clone()
                })
            } else {
                req.clone()
            };
            (rt.callback)(packet)
        }
    };

    let val_l = res_str.len();
    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let alloc_res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = alloc_res[0].to_i32();

    let arr = res_str.as_bytes().to_vec();
    let mut mem2 = _inst.get_memory_mut("memory").unwrap();
    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();

    let c = ((val_offset as i64) << 32) | (val_l as i64);
    Ok(vec![WasmValue::from_i64(c)])
}

pub fn output(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_ref(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };

    let key_offset = _input[0].to_i32();
    let key_l = _input[1].to_i32();
    let text_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let text_bytes_next = text_bytes.unwrap();
    let text = str::from_utf8(&text_bytes_next).unwrap();

    rt.execution_result = text.to_string();
    rt.has_output = true;

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn console_log(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let mem = _caller.memory_ref(0).unwrap();

    let key_offset = _input[0].to_i32();
    let key_l = _input[1].to_i32();
    let text_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let text_bytes_next = text_bytes.unwrap();
    let text = str::from_utf8(&text_bytes_next).unwrap();

    log_vm(text.to_string(), rt.vm_id.clone(), "runtime");

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn plant_trigger(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let tag = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let text = str::from_utf8(&key_bytes_next).unwrap();

    let pi_offset = _input[4].to_i32();
    let pi_l = _input[5].to_i32();
    let pi_bytes = mem.get_data(pi_offset.cast_unsigned(), pi_l.cast_unsigned());
    let pi_bytes_next = pi_bytes.unwrap();
    let store_id = str::from_utf8(&pi_bytes_next).unwrap();

    let count = _input[6].to_i32();

    let j = json!({
        "key": "plantTrigger",
        "input": {
            "machineId": rt.machine_id,
            "storeId": store_id,
            "input": text,
            "tag": tag,
            "count": count
        }
    });

    (rt.callback)(j);

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn http_post(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let url_offset = _input[0].to_i32();
    let url_l = _input[1].to_i32();
    let url_bytes = mem.get_data(url_offset.cast_unsigned(), url_l.cast_unsigned());
    let url_bytes_next = url_bytes.unwrap();
    let url = str::from_utf8(&url_bytes_next).unwrap();

    let heads_offset = _input[2].to_i32();
    let heads_l = _input[3].to_i32();
    let heads_bytes = mem.get_data(heads_offset.cast_unsigned(), heads_l.cast_unsigned());
    let heads_bytes_next = heads_bytes.unwrap();
    let headers = str::from_utf8(&heads_bytes_next).unwrap();

    let body_offset = _input[4].to_i32();
    let body_l = _input[5].to_i32();
    let body_bytes = mem.get_data(body_offset.cast_unsigned(), body_l.cast_unsigned());
    let body_bytes_next = body_bytes.unwrap();
    let body = str::from_utf8(&body_bytes_next).unwrap();

    let j = json!({
        "key": "httpPost",
        "input": {
            "machineId": rt.machine_id,
            "url": url,
            "headers": headers,
            "body": body
        }
    });

    let val = (rt.callback)(j);
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn run_docker(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let mem = _inst.get_memory_mut("memory").unwrap();

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let image_name = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let text = str::from_utf8(&key_bytes_next).unwrap();

    let cn_offset = _input[4].to_i32();
    let cn_l = _input[5].to_i32();
    let cn_bytes = mem.get_data(cn_offset.cast_unsigned(), cn_l.cast_unsigned());
    let cn_bytes_next = cn_bytes.unwrap();
    let container_name = str::from_utf8(&cn_bytes_next).unwrap();

    let j = json!({
        "key": "runVm",
        "input": {
            "runtime": "docker",
            "machineId": rt.machine_id,
            "storeId": rt.store_id,
            "inputFiles": text,
            "imageName": image_name,
            "containerName": container_name
        }
    });

    let val = (rt.callback)(j);

    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn exec_docker(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let image_name = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let container_name = str::from_utf8(&key_bytes_next).unwrap();

    let cn_offset = _input[4].to_i32();
    let cn_l = _input[5].to_i32();
    let cn_bytes = mem.get_data(cn_offset.cast_unsigned(), cn_l.cast_unsigned());
    let cn_bytes_next = cn_bytes.unwrap();
    let command = str::from_utf8(&cn_bytes_next).unwrap();

    let j = json!({
        "key": "execDocker",
        "input": {
            "machineId": rt.machine_id,
            "imageName": image_name,
            "containerName": container_name,
            "command": command
        }
    });

    let res = (rt.callback)(j);

    let jres = json!({
        "data": res
    });

    let val = jres.to_string();
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn copy_to_docker(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let image_name = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let container_name = str::from_utf8(&key_bytes_next).unwrap();

    let co_offset = _input[4].to_i32();
    let co_l = _input[5].to_i32();
    let co_bytes = mem.get_data(co_offset.cast_unsigned(), co_l.cast_unsigned());
    let co_bytes_next = co_bytes.unwrap();
    let file_name = str::from_utf8(&co_bytes_next).unwrap();

    let cn_offset = _input[6].to_i32();
    let cn_l = _input[7].to_i32();
    let cn_bytes = mem.get_data(cn_offset.cast_unsigned(), cn_l.cast_unsigned());
    let cn_bytes_next = cn_bytes.unwrap();
    let content = str::from_utf8(&cn_bytes_next).unwrap();

    let j = json!({
        "key": "copyToDocker",
        "input": {
            "machineId": rt.machine_id,
            "imageName": image_name,
            "containerName": container_name,
            "fileName": file_name,
            "content": content
        }
    });

    let res = (rt.callback)(j);

    let jres = json!({
        "data": res
    });

    let val = jres.to_string();
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn signal_store(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let typ = str::from_utf8(&in_bytes_next).unwrap();

    let key_offset = _input[2].to_i32();
    let key_l = _input[3].to_i32();
    let key_bytes = mem.get_data(key_offset.cast_unsigned(), key_l.cast_unsigned());
    let key_bytes_next = key_bytes.unwrap();
    let store_id = str::from_utf8(&key_bytes_next).unwrap();

    let co_offset = _input[4].to_i32();
    let co_l = _input[5].to_i32();
    let co_bytes = mem.get_data(co_offset.cast_unsigned(), co_l.cast_unsigned());
    let co_bytes_next = co_bytes.unwrap();
    let user_id = str::from_utf8(&co_bytes_next).unwrap();

    let cn_offset = _input[6].to_i32();
    let cn_l = _input[7].to_i32();
    let cn_bytes = mem.get_data(cn_offset.cast_unsigned(), cn_l.cast_unsigned());
    let cn_bytes_next = cn_bytes.unwrap();
    let payload = str::from_utf8(&cn_bytes_next).unwrap();

    let j = json!({
        "key": "signal",
        "input": {
            "machineId": rt.machine_id,
            "type": typ,
            "storeId": store_id,
            "userId": user_id,
            "data": payload
        }
    });

    let val = (rt.callback)(j);
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn trx_put(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let key = str::from_utf8(&in_bytes_next).unwrap();

    let val_offset = _input[2].to_i32();
    let val_l = _input[3].to_i32();
    let val_bytes = mem.get_data(val_offset.cast_unsigned(), val_l.cast_unsigned());
    let val_bytes_next = val_bytes.unwrap();
    let val = str::from_utf8(&val_bytes_next).unwrap();

    rt.trx
        .put(format!("{}::{}", rt.machine_id, key), val.to_string());

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn trx_del(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let key = str::from_utf8(&in_bytes_next).unwrap();

    rt.trx.del(format!("{}::{}", rt.machine_id, key));

    Ok(vec![WasmValue::from_i64(0)])
}

pub fn trx_get(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let key = str::from_utf8(&in_bytes_next).unwrap();

    let val = rt.trx.get(format!("{}::{}", rt.machine_id, key));
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}

pub fn trx_get_by_prefix(
    host_data: &mut HostData,
    _inst: &mut Instance,
    _caller: &mut CallingFrame,
    _input: Vec<WasmValue>,
) -> Result<Vec<WasmValue>, CoreError> {
    let mem = _caller.memory_mut(0).unwrap();
    let rt: &mut WasmMac = unsafe { &mut *host_data.runtime };
    let exec: &mut Executor = unsafe { &mut *host_data.exec };

    let in_offset = _input[0].to_i32();
    let in_l = _input[1].to_i32();
    let in_bytes = mem.get_data(in_offset.cast_unsigned(), in_l.cast_unsigned());
    let in_bytes_next = in_bytes.unwrap();
    let prefix = str::from_utf8(&in_bytes_next).unwrap();

    let vals = rt
        .trx
        .get_by_prefix(format!("{}::{}", rt.machine_id, prefix));
    let j = json!({
        "data": vals
    });

    let val = j.to_string();
    let val_l = val.len();

    let mut malloc_fn = _inst.get_func_mut("malloc").unwrap();
    let mfn = malloc_fn.deref_mut();
    let res = exec
        .call_func(mfn, [WasmValue::from_i32(val_l as i32)])
        .unwrap();
    let val_offset = res[0].to_i32();

    let arr = val.as_bytes().to_vec();

    let mut mem2 = _inst.get_memory_mut("memory").unwrap();

    mem2.set_data(arr, val_offset.cast_unsigned()).unwrap();
    let c = ((val_offset as i64) << 32) | (val_l as i64);

    Ok(vec![WasmValue::from_i64(c)])
}
