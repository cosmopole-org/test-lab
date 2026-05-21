use crate::prelude::*;
use crate::host::functions::protocol_api::forward_host_api_packet;

pub(crate) fn host_fn_create_creature(input: &JsonValue) -> String {
    forward_host_api_packet("createCreature", input)
}

pub(crate) fn host_fn_delete_creature(input: &JsonValue) -> String {
    forward_host_api_packet("deleteCreature", input)
}
