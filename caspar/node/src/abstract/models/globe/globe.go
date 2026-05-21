package globe

import (
	"kasper/src/abstract/models/chain"
	"kasper/src/abstract/models/update"
	"time"
)

type IGlobe interface {
	SendBaseRequestOnChain(key string, payload []byte, signature string, userId string, tag string, callback func([]byte, int, error))
	SendTypedMessageOnChain(chainId string, key string, messageType string, payload []byte, signature string, userId string, receivers map[string]map[string]bool, replyTo string, storeId string, pay *chain.ChainPayPacket, callback func(string, []byte))
	ExecBaseResponseOnChain(callbackId string, packet []byte, signature string, resCode int, e string, updates []update.Update, tag string, toUserId string)
	Handle(typ string, trxPayload []byte) bool
	StakeNodeOwner(nodeId string, ownerId string, amount int64)
	TryStartScheduledElection(now time.Time)
}
