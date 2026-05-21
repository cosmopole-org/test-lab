use crate::prelude::*;
use crate::host::vm_host_functions::{with_docker_controller, with_fire_controller};

pub fn restore_previously_running_vms(snapshot: &JsonValue) -> Result<JsonValue, String> {
    let runtimes = snapshot["vms"].as_array().cloned().unwrap_or_default();
    let mut restored = 0;
    for vm in runtimes {
        let runtime = vm["runtime"].as_str().unwrap_or("wasm");
        if runtime == "docker" {
            let _ = with_docker_controller(|controller| controller.run_vm(&vm));
        } else if runtime == "fire" {
            let _ = with_fire_controller(|controller| controller.run_vm(&vm));
        }
        restored += 1;
    }
    Ok(json!({"ok": true, "restored": restored}))
}
