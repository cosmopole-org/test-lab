package vmm

import "kasper/src/abstract/models/worker"

type IVmm interface {
	Assign(machineId string)
	RunVm(machineId string, storeId string, data string)
	TerminateVm(machineId string)
	BuildVmImage(machineId string, entityId string, buildPath string, buildType string)
	ExecuteChainTrxsGroup(trxs []*worker.Trx)
	ExecuteChainEffects(effects string)
	CloseKVDB()
	VmCallback(dataRaw string) (string, int64)
}
