use crate::prelude::*;
use crate::host::vm_host_functions::{HostHierarchy, run_db_op};

pub(crate) fn host_fn_db_op(ctx: &HostHierarchy, input: &JsonValue) -> String {
    match run_db_op(ctx, input) {
        Ok(res) => res,
        Err(err) => json!({"ok": false, "error": err}).to_string(),
    }
}
