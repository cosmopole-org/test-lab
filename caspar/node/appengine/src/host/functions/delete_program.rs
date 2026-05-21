use crate::prelude::*;
use crate::host::functions::protocol_api::forward_host_api_packet;

pub(crate) fn host_fn_delete_program(input: &JsonValue) -> String {
    forward_host_api_packet("deleteProgram", input)
}
