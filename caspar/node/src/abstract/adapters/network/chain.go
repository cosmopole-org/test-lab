package network

import "crypto/tls"

type IChain interface {
	Listen(port int, tlsConfig *tls.Config)
	RestoreFromStorage()
	SubmitTrx(chainId string, machineId string, typ string, payload []byte)
	RegisterPipeline(pipeline func([][]byte, func([]byte)) []string)
	NotifyNewMachineCreated(chainId string, machineId string)
	CreateTempChain(storeId string) string
	CreateWorkChain(storeId string) string
	CreateShardChain(chainId string, shardChainId string, peers []string) string
	Peers() []string
	UserOwnsOrigin(userId string, origin string) bool
	GetNodeOwnerId(origin string) string
	Close()
}
