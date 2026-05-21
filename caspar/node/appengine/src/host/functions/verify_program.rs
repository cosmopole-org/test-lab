use crate::prelude::*;
use crate::models::vm_runtime::{verify_program_execution_from_packet, parse_u64_array_field, parse_u8_array_field};

pub(crate) fn host_fn_verify_program(input: &JsonValue) -> String {
    let masm_path = input["masmPath"].as_str().unwrap_or("").to_string();
    let inputs = parse_u64_array_field(input, "inputs");
    let outputs = parse_u64_array_field(input, "outputs");
    let proof_bytes = parse_u8_array_field(input, "proof");

    match verify_program_execution_from_packet(&masm_path, &inputs, &outputs, &proof_bytes) {
        Ok(security) => json!({"ok": true, "security": security}).to_string(),
        Err(err) => json!({"ok": false, "error": err}).to_string(),
    }
}
