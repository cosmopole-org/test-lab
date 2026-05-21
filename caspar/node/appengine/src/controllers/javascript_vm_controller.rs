use crate::prelude::*;
use crate::controllers::vm_controller::VmController;
use crate::controllers::wasm_vm_controller::WasmVmController;

pub(crate) struct JavascriptVmController;

impl VmController for JavascriptVmController {
    fn build_image(_packet: &JsonValue) -> Result<JsonValue, String> {
        Ok(json!({"ok": true, "runtime": "javascript", "build": "noop"}))
    }

    fn create(packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        Ok(json!({"ok": true, "runtime": "javascript", "machineId": machine_id}))
    }

    fn starts(packet: &JsonValue) -> Result<JsonValue, String> {
        Self::create(packet)
    }
    fn stop(packet: &JsonValue) -> Result<JsonValue, String> {
        WasmVmController::stop(packet)
    }
    fn resume(packet: &JsonValue) -> Result<JsonValue, String> {
        Self::starts(packet)
    }
    fn pause(packet: &JsonValue) -> Result<JsonValue, String> {
        Self::stop(packet)
    }

    fn exec(packet: &JsonValue) -> Result<JsonValue, String> {
        let script_path = packet["astPath"].as_str().unwrap_or("");
        if script_path.is_empty() {
            return Err("astPath is required for javascript runtime".to_string());
        }
        let _ = transpile_js_to_masm(script_path)
            .map_err(|e| format!("javascript transpile failed: {}", e))?;
        Ok(json!({"ok": true, "runtime": "javascript", "astPath": script_path}))
    }

    fn copy_to(_packet: &JsonValue) -> Result<JsonValue, String> {
        Err("copy_to is not implemented yet for javascript runtime".to_string())
    }

    fn copy_from(_packet: &JsonValue) -> Result<JsonValue, String> {
        Err("copy_from is not implemented yet for javascript runtime".to_string())
    }
}
