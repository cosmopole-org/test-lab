use crate::prelude::*;

#[derive(Clone, Debug, Default)]
pub(crate) struct BaseVm {
    pub(crate) machine_id: String,
    pub(crate) vm_id: String,
    pub(crate) runtime: String,
    pub(crate) status: String,
    pub(crate) requester_user_id: String,
    pub(crate) store_id: String,
    pub(crate) created_at_unix_ms: i64,
    pub(crate) updated_at_unix_ms: i64,
}

impl BaseVm {
    pub(crate) fn from_packet(packet: &JsonValue, runtime: &str) -> Self {
        let now = 0_i64;
        BaseVm {
            machine_id: packet["machineId"].as_str().unwrap_or("").to_string(),
            vm_id: packet["vmId"].as_str().unwrap_or("main").to_string(),
            runtime: runtime.to_string(),
            status: "created".to_string(),
            requester_user_id: packet["requesterUserId"].as_str().unwrap_or("").to_string(),
            store_id: packet["storeId"].as_str().unwrap_or("").to_string(),
            created_at_unix_ms: now,
            updated_at_unix_ms: now,
        }
    }
}
