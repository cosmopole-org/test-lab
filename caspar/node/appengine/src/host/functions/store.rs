use crate::prelude::*;
use crate::host::functions::protocol_api::forward_host_api_packet;

pub(crate) fn host_fn_create_store(input: &JsonValue) -> String {
    forward_host_api_packet("createStore", input)
}

pub(crate) fn host_fn_delete_store(input: &JsonValue) -> String {
    forward_host_api_packet("deleteStore", input)
}
