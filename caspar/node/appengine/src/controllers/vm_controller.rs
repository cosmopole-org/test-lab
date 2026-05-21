use crate::prelude::*;

pub(crate) trait VmController {
    fn build_image(packet: &JsonValue) -> Result<JsonValue, String>;
    fn create(packet: &JsonValue) -> Result<JsonValue, String>;
    fn starts(packet: &JsonValue) -> Result<JsonValue, String>;
    fn stop(packet: &JsonValue) -> Result<JsonValue, String>;
    fn resume(packet: &JsonValue) -> Result<JsonValue, String>;
    fn pause(packet: &JsonValue) -> Result<JsonValue, String>;
    fn exec(packet: &JsonValue) -> Result<JsonValue, String>;
    fn copy_to(packet: &JsonValue) -> Result<JsonValue, String>;
    fn copy_from(packet: &JsonValue) -> Result<JsonValue, String>;
}
