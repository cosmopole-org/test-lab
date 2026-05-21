use crate::prelude::*;

pub(crate) struct VmNetworkService;

impl VmNetworkService {
    pub(crate) fn gateway_network_name() -> &'static str {
        "kasper"
    }

    pub(crate) fn firecracker_socket(machine_id: &str, vm_id: &str) -> String {
        format!("/opt/firecracker/vms/fc.{}.{}.sock", machine_id, vm_id)
    }

    pub(crate) fn vm_http_gateway_url(machine_id: &str, vm_id: &str, port: u16) -> String {
        format!(
            "http://127.0.0.1:8080/vm/{}/{}/{}",
            machine_id.trim(),
            vm_id.trim(),
            port
        )
    }
}
