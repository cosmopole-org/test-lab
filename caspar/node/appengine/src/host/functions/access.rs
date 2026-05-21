use crate::prelude::*;
use crate::host::functions::protocol_api::forward_host_api_packet;

pub(crate) fn host_fn_create_access(input: &JsonValue) -> String {
    forward_host_api_packet("createAccess", input)
}

pub(crate) fn host_fn_delete_access(input: &JsonValue) -> String {
    forward_host_api_packet("deleteAccess", input)
}
