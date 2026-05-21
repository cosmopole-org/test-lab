package tool_storage

import (
	"context"
	"encoding/binary"
	"fmt"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/packet"
	"kasper/src/abstract/models/trx"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/google/uuid"

	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type StorageManager struct {
	core        core.ICore
	storageRoot string
	kvdb        *badger.DB
	tsdb        *sql.DB
	lock        sync.Mutex
}

func (sm *StorageManager) StorageRoot() string {
	return sm.storageRoot
}
func (sm *StorageManager) KvDb() *badger.DB {
	return sm.kvdb
}
func (sm *StorageManager) TsDb() *sql.DB {
	return sm.tsdb
}

func (sm *StorageManager) LogVm(vmId string, logType string, data string, timeVal int64) packet.BuildPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	id := uuid.NewString()
	if logType == "" {
		logType = "runtime"
	}
	if timeVal == 0 {
		timeVal = time.Now().UnixMilli()
	}
	_, err := sm.tsdb.ExecContext(ctx,
		"INSERT INTO buildlogs (id, build_id, machine_id, vm_id, log_type, data, time) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		id, "", "", vmId, logType, data, timeVal,
	)
	if err != nil {
		log.Println("Insert error: " + err.Error())
	}
	packet := packet.BuildPacket{Id: id, BuildId: "", CreatureId: "", VmId: vmId, LogType: logType, Time: timeVal, Data: data}
	return packet
}

func (sm *StorageManager) ReadVmLogs(vmId string, logType string, offset int, count int) []packet.BuildPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	if count <= 0 {
		count = 100
	}
	if offset < 0 {
		offset = 0
	}
	query := "SELECT id, build_id, machine_id, vm_id, log_type, data, time FROM buildlogs WHERE vm_id = $1"
	args := []any{vmId}
	if logType != "" {
		args = append(args, logType)
		query += fmt.Sprintf(" AND log_type = $%d", len(args))
	}
	args = append(args, count)
	query += fmt.Sprintf(" ORDER BY time DESC LIMIT $%d", len(args))
	args = append(args, offset)
	query += fmt.Sprintf(" OFFSET $%d", len(args))
	rows, err := sm.tsdb.QueryContext(ctx, query, args...)
	if err != nil {
		log.Println(err)
		return []packet.BuildPacket{}
	}
	defer rows.Close()
	logs := []packet.BuildPacket{}
	for rows.Next() {
		var id string
		var logBuildId string
		var logCreatureId string
		var logVmId string
		var rowLogType string
		var data string
		var timeVal int64
		if err := rows.Scan(&id, &logBuildId, &logCreatureId, &logVmId, &rowLogType, &data, &timeVal); err != nil {
			log.Println(err)
		}
		logs = append(logs, packet.BuildPacket{
			Id:         id,
			BuildId:    logBuildId,
			CreatureId: logCreatureId,
			VmId:       logVmId,
			LogType:    rowLogType,
			Time:       timeVal,
			Data:       data,
		})
	}
	return logs
}

func (sm *StorageManager) LogTimeSieries(storeId string, userId string, data string, timeVal int64) packet.LogPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	id := uuid.NewString()
	_, err := sm.tsdb.ExecContext(ctx,
		"INSERT INTO storage (id, store_id, user_id, data, time, edited) VALUES ($1, $2, $3, $4, $5, $6)",
		id, storeId, userId, data, timeVal, false,
	)
	if err != nil {
		log.Println("Insert error: " + err.Error())
	}
	packet := packet.LogPacket{Id: id, UserId: userId, Data: data, StoreId: storeId, Time: timeVal, Edited: false}
	return packet
}

func (sm *StorageManager) UpdateLog(storeId string, userId string, signalId string, data string, timeVal int64) packet.LogPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	_, err := sm.tsdb.ExecContext(ctx,
		"update storage set data = $1 where store_id = $2 and id = $3 and edited = $4",
		data, storeId, signalId, true,
	)
	if err != nil {
		log.Println("Update error: " + err.Error())
	}
	packet := packet.LogPacket{Id: signalId, UserId: userId, Data: data, StoreId: storeId, Time: timeVal, Edited: true}
	return packet
}

func (sm *StorageManager) ReadStoreLogs(storeId string, beforeTime int64, count int) []packet.LogPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	var rows *sql.Rows
	var err error
	if beforeTime == 0 {
		rows, err = sm.tsdb.QueryContext(ctx, "SELECT id, user_id, data, time, edited FROM storage WHERE store_id = $1 order by time desc limit $2", storeId, count)
		if err != nil {
			log.Println(err)
			return []packet.LogPacket{}
		}
		defer rows.Close()
	} else {
		rows, err = sm.tsdb.QueryContext(ctx, "SELECT id, user_id, data, time, edited FROM storage WHERE store_id = $1 and time < $2 order by time desc limit $3", storeId, beforeTime, count)
		if err != nil {
			log.Println(err)
			return []packet.LogPacket{}
		}
		defer rows.Close()
	}
	logs := []packet.LogPacket{}

	for rows.Next() {
		var id string
		var userId string
		var data string
		var timeVal int64
		var edited bool
		if err := rows.Scan(&id, &userId, &data, &timeVal, &edited); err != nil {
			log.Println(err)
		}
		logs = append(logs, packet.LogPacket{Id: id, UserId: userId, Data: data, StoreId: storeId, Time: timeVal, Edited: edited})
	}
	return logs
}

func (sm *StorageManager) PickStoreLogs(storeId string, ids []string) []packet.LogPacket {

	ctx := context.Background()
	if len(ids) == 0 {
		return []packet.LogPacket{}
	}
	rows, err := sm.tsdb.QueryContext(ctx, "SELECT id, user_id, data, time, edited FROM storage WHERE store_id = $1 and id in ('"+strings.Join(ids, "','")+"')", storeId)
	if err != nil {
		log.Println(err)
		return []packet.LogPacket{}
	}
	defer rows.Close()
	logs := []packet.LogPacket{}
	fmt.Println("Query results:")
	for rows.Next() {
		var id string
		var userId string
		var data string
		var timeVal int64
		var edited bool
		if err := rows.Scan(&id, &userId, &data, &timeVal, &edited); err != nil {
			log.Println(err)
		}
		logs = append(logs, packet.LogPacket{Id: id, UserId: userId, Data: data, StoreId: storeId, Time: timeVal, Edited: edited})
	}
	return logs
}

func (sm *StorageManager) GenId(t trx.ITrx, origin string) string {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if origin == "global" {
		item := t.GetBytes("globalIdCounter")
		var counter int64 = 0
		if len(item) == 0 {
			counter = 0
		} else {
			counter = int64(binary.BigEndian.Uint64(item))
		}
		counter++
		nextB := [8]byte{}
		binary.BigEndian.PutUint64(nextB[:], uint64(counter))
		t.PutBytes("globalIdCounter", nextB[:])
		return fmt.Sprintf("%d@%s", counter, origin)
	} else {
		trx := sm.kvdb.NewTransaction(true)
		defer trx.Commit()
		item, err := trx.Get([]byte("localIdCounter"))
		var counter int64 = 0
		if err != nil {
			counter = 0
		} else {
			var b []byte
			item.Value(func(val []byte) error {
				b = val
				return nil
			})
			counter = int64(binary.BigEndian.Uint64(b))
		}
		counter++
		nextB := [8]byte{}
		binary.BigEndian.PutUint64(nextB[:], uint64(counter))
		trx.Set([]byte("localIdCounter"), nextB[:])
		return fmt.Sprintf("%d@%s", counter, origin)
	}
}

func NewStorage(core core.ICore, storageRoot string, baseDbPath string, logsDbPath string, searcherDbPath string) *StorageManager {
	log.Println("connecting to database...")
	os.MkdirAll(baseDbPath, os.ModePerm)
	kvdb, err := badger.Open(badger.DefaultOptions(baseDbPath).WithSyncWrites(true))
	if err != nil {
		panic(err)
	}
	tsdb, err := sql.Open("pgx", "postgres://admin:quest@localhost:8812/qdb?sslmode=disable")
	if err != nil {
		panic(err)
	}
	for {
		_, err = tsdb.ExecContext(context.Background(),
			"create table if not exists storage(id text, store_id text, user_id text, data text, time bigint, edited boolean);",
		)
		if err != nil {
			log.Println(err)
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}
	_, err = tsdb.ExecContext(context.Background(),
		"create table if not exists buildlogs(id text, build_id text, machine_id text, vm_id text, log_type text, data text, time bigint);",
	)
	if err != nil {
		panic(err)
	}
	_, _ = tsdb.ExecContext(context.Background(), "alter table buildlogs add column if not exists vm_id text;")
	_, _ = tsdb.ExecContext(context.Background(), "alter table buildlogs add column if not exists log_type text;")
	_, _ = tsdb.ExecContext(context.Background(), "alter table buildlogs add column if not exists time bigint;")
	return &StorageManager{core: core, tsdb: tsdb, kvdb: kvdb, storageRoot: storageRoot}
}
