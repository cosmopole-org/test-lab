use crate::prelude::*;
use crate::models::base_vm::BaseVm;

#[derive(Clone, Debug)]
pub(crate) struct ElpianVm {
    pub(crate) base: BaseVm,
    pub(crate) ast_path: String,
}
