use crate::prelude::*;
use crate::models::base_vm::BaseVm;

#[derive(Clone, Debug)]
pub(crate) struct DockerVm {
    pub(crate) base: BaseVm,
    pub(crate) container_id: String,
    pub(crate) image_ref: String,
    pub(crate) env: Vec<String>,
}
