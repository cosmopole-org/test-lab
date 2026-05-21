use crate::prelude::*;
use crate::host::functions::protocol_api::forward_host_api_packet;

pub(crate) fn host_fn_lock_token(input: &JsonValue) -> String {
    forward_host_api_packet("lockToken", input)
}
