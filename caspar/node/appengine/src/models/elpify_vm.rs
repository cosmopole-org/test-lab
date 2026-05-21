use crate::prelude::*;
use crate::models::base_vm::BaseVm;

#[derive(Clone, Debug)]
pub(crate) struct ElpifyVm {
    pub(crate) base: BaseVm,
    pub(crate) masm_path: String,
}
