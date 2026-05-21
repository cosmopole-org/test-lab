use crate::prelude::*;
use crate::controllers::vm_controller::VmController;
use crate::models::vm_runtime::{execute_elpian_task, terminate_managed_vm, parse_vm_resource_limits};

pub(crate) struct ElpianVmController;

impl VmController for ElpianVmController {
    fn build_image(_packet: &JsonValue) -> Result<JsonValue, String> {
        Ok(json!({"ok": true, "runtime": "elpian", "build": "noop"}))
    }

    fn create(packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        Ok(json!({"ok": true, "runtime": "elpian", "machineId": machine_id}))
    }

    fn starts(packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        let vm_id = packet["vmId"].as_str().unwrap_or("main").to_string();
        let ast_path = packet["astPath"].as_str().unwrap_or("").to_string();
        let input = packet["input"].as_str().unwrap_or("{}").to_string();
        let limits = parse_vm_resource_limits(packet);
        execute_elpian_task(machine_id, vm_id, ast_path, input, limits)?;
        Ok(json!({"ok": true, "runtime": "elpian", "machineId": machine_id}))
    }

    fn stop(packet: &JsonValue) -> Result<JsonValue, String> {
        let machine_id = packet["machineId"].as_str().unwrap_or("");
        if machine_id.is_empty() {
            return Err("machineId is required".to_string());
        }
        terminate_managed_vm(machine_id);
        Ok(json!({"ok": true, "runtime": "elpian", "machineId": machine_id}))
    }

    fn resume(packet: &JsonValue) -> Result<JsonValue, String> {
        Self::starts(packet)
    }
    fn pause(packet: &JsonValue) -> Result<JsonValue, String> {
        Self::stop(packet)
    }
    fn exec(packet: &JsonValue) -> Result<JsonValue, String> {
        Self::starts(packet)
    }

    fn copy_to(_packet: &JsonValue) -> Result<JsonValue, String> {
        Err("copy_to is not implemented yet for elpian runtime".to_string())
    }

    fn copy_from(_packet: &JsonValue) -> Result<JsonValue, String> {
        Err("copy_from is not implemented yet for elpian runtime".to_string())
    }
}
