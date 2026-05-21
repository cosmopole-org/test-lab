use crate::prelude::*;
use crate::controllers::vm_controller::VmController;
use crate::models::vm_runtime::terminate_managed_vm;

pub(crate) struct WasmVmController;

impl VmController for WasmVmController {
    fn build_image(_packet: &JsonValue) -> Result<JsonValue, String> {
        Ok(json!({"ok": true, "runtime": "wasm", "build": "noop"}))
    }

    fn create(packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        Ok(json!({"ok": true, "runtime": "wasm", "machineId": machine_id}))
    }

    fn starts(packet: &JsonValue) -> Result<JsonValue, String> {
        Self::create(packet)
    }

    fn stop(packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        terminate_managed_vm(machine_id);
        Ok(json!({"ok": true, "runtime": "wasm", "machineId": machine_id}))
    }

    fn resume(packet: &JsonValue) -> Result<JsonValue, String> {
        Self::starts(packet)
    }

    fn pause(packet: &JsonValue) -> Result<JsonValue, String> {
        Self::stop(packet)
    }

    fn exec(packet: &JsonValue) -> Result<JsonValue, String> {
        let ast_path = packet["astPath"].as_str().unwrap_or("").to_string();
        if ast_path.is_empty() {
            return Err("astPath is required".to_string());
        }
        Ok(json!({"ok": true, "runtime": "wasm", "astPath": ast_path}))
    }

    fn copy_to(_packet: &JsonValue) -> Result<JsonValue, String> {
        Err("copy_to is not implemented yet for wasm runtime".to_string())
    }

    fn copy_from(_packet: &JsonValue) -> Result<JsonValue, String> {
        Err("copy_from is not implemented yet for wasm runtime".to_string())
    }
}
