package core

import (
	"kasper/src/abstract/adapters/tools"
	"kasper/src/abstract/models/action"
	"kasper/src/abstract/models/chain"
	"kasper/src/abstract/models/globe"
	"kasper/src/abstract/models/info"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/state"
)

type EmptyPayload struct{}

type ResponseHolder struct {
	Payload []byte
	Effects chain.Effects
}

type ICore interface {
	OwnerId() string
	Id() string
	Gods() []string
	AddGod(username string)
	Tools() tools.ITools
	FreeNodes() map[string]bool
	AddFreeNode(nodeId string)
	Actor() action.IActor
	Load([]string, map[string]interface{})
	Close()
	PlantChainTrigger(count int, userId string, tag string, machineId string, storeId string, input string)
	AppPendingTrxs()
	IpAddr() string
	ModifyState(bool, func(trx.ITrx) error)
	ModifyStateSecurlyWithSource(readonly bool, info info.IInfo, src string, fn func(state.IState) error)
	ModifyStateSecurly(readonly bool, info info.IInfo, fn func(state.IState) error)
	SignPacket(data []byte) string
	SignPacketAsOwner(data []byte) string
	ExecutionCostPerSecond() int64
	VmRamCostPerMbPerMinute() int64
	VmCpuCoreCostPerMinute() int64
	VmDiskCostPerGbPerMinute() int64
	Globe() globe.IGlobe
}
