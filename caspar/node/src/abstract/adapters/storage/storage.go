package storage

import (
	"kasper/src/abstract/models/packet"
	"kasper/src/abstract/models/trx"

	"database/sql"
	"github.com/dgraph-io/badger"
)

type IStorage interface {
	StorageRoot() string
	KvDb() *badger.DB
	TsDb() *sql.DB
	GenId(t trx.ITrx, origin string) string
	LogTimeSieries(storeId string, userId string, data string, timeVal int64) packet.LogPacket
	UpdateLog(storeId string, userId string, signalId string, data string, timeVal int64) packet.LogPacket
	ReadStoreLogs(storeId string, beforeTime int64, count int) []packet.LogPacket
	PickStoreLogs(storeId string, ids []string) []packet.LogPacket
	LogVm(vmId string, logType string, data string, timeVal int64) packet.BuildPacket
	ReadVmLogs(vmId string, logType string, offset int, count int) []packet.BuildPacket
}
