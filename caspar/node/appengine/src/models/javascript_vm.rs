use crate::prelude::*;
use crate::models::base_vm::BaseVm;

#[derive(Clone, Debug)]
pub(crate) struct JavascriptVm {
    pub(crate) base: BaseVm,
    pub(crate) script_path: String,
    pub(crate) transpiled_masm_path: String,
}
