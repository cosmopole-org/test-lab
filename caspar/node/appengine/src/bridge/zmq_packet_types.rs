use crate::prelude::*;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum ZmqPacketType {
    RunVm,
    TerminateVm,
    ExecVm,
    CopyToVm,
    BuildVmImage,
    HostCall,
    VerifyProgramExecution,
    ApiResponse,
    BridgeApi,
    Unknown,
}

impl ZmqPacketType {
    pub(crate) fn from_packet(packet: &JsonValue) -> Self {
        match packet["type"].as_str().unwrap_or("") {
            "runVm" => Self::RunVm,
            "terminateVm" => Self::TerminateVm,
            "execVm" | "execDocker" => Self::ExecVm,
            "copyToVm" | "copyToDocker" => Self::CopyToVm,
            "buildVmImage" | "buildDockerImage" => Self::BuildVmImage,
            "hostCall" => Self::HostCall,
            "verifyProgramExecution" | "elpifyProof" => Self::VerifyProgramExecution,
            "apiResponse" => Self::ApiResponse,
            "bridgeApi" => Self::BridgeApi,
            _ => Self::Unknown,
        }
    }
}

#[derive(Clone, Debug)]
pub(crate) struct ZmqPacketEnvelope {
    pub(crate) packet_type: ZmqPacketType,
    pub(crate) runtime: String,
    pub(crate) machine_id: String,
    pub(crate) vm_id: String,
}

impl ZmqPacketEnvelope {
    pub(crate) fn from_packet(packet: &JsonValue) -> Self {
        ZmqPacketEnvelope {
            packet_type: ZmqPacketType::from_packet(packet),
            runtime: packet["runtime"].as_str().unwrap_or("").to_lowercase(),
            machine_id: packet["machineId"].as_str().unwrap_or("").to_string(),
            vm_id: packet["vmId"].as_str().unwrap_or("main").to_string(),
        }
    }
}
