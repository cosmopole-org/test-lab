use crate::prelude::*;
use crate::host::vm_host_functions::{with_docker_controller, with_fire_controller};

pub(crate) fn host_fn_run_vm(input: &JsonValue) -> String {
    let runtime = input["runtime"].as_str().unwrap_or("").to_lowercase();
    let result = if runtime == "fire" {
        with_fire_controller(|controller| controller.run_vm(input))
    } else {
        with_docker_controller(|controller| controller.run_vm(input))
    };
    match result {
        Ok(res) => res.to_string(),
        Err(err) => json!({"ok": false, "error": err}).to_string(),
    }
}
