use crate::prelude::*;
use crate::host::functions::protocol_api::forward_host_api_packet;

pub(crate) fn host_fn_validate_sign(input: &JsonValue) -> String {
    forward_host_api_packet("validateSign", input)
}
