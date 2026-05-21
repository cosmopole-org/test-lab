package vmm

import (
	"encoding/json"
	"errors"
	"fmt"
	"kasper/src/abstract/adapters/file"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/adapters/storage"
	"kasper/src/abstract/models/chain"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/models/worker"
	"kasper/src/abstract/state"
	"kasper/src/core/module/actor/model/base"
	inputs_stores "kasper/src/shell/api/inputs/stores"
	model "kasper/src/shell/api/model"
	updates_stores "kasper/src/shell/api/updates/stores"
	"kasper/src/shell/utils/future"
	"log"
	"os"
	"strings"
	"time"

	zmq "github.com/pebbe/zmq4"
)

type Vmm struct {
	app         core.ICore
	storageRoot string
	storage     storage.IStorage
	file        file.IFile
	aeSocket    chan string
}

func (wm *Vmm) Assign(machineId string) {
	wm.app.Tools().Signaler().ListenToSingle(&signaler.Listener{
		Id: machineId,
		Signal: func(key string, a any) {
			if key != "creatures/signal" {
				return
			}
			raw, _ := a.([]byte)
			data := string(raw)
			var pkt updates_stores.Send
			entityId := ""
			if json.Unmarshal(raw, &pkt) == nil {
				entityId = pkt.EntityId
			}
			astPath, vmType := wm.resolveVmExecutionTarget(machineId, entityId)
			str, _ := json.Marshal(map[string]any{
				"type":      "runVm",
				"machineId": machineId,
				"input":     data,
				"astPath":   astPath,
				"vmType":    vmType,
			})
			wm.aeSocket <- string(str)
		},
	})
}

func (wm *Vmm) ExecuteChainTrxsGroup(trxs []*worker.Trx) {
	_ = trxs
}

func (wm *Vmm) ExecuteChainEffects(effects string) {
	_ = effects
}

type ChainDbOp struct {
	OpType string `json:"opType"`
	Key    string `json:"key"`
	Val    string `json:"val"`
}

func normalizeRuntime(runtime string) string {
	return strings.ToLower(strings.TrimSpace(runtime))
}

func isManagedRuntime(runtime string) bool {
	runtime = normalizeRuntime(runtime)
	return runtime == "wasm" || runtime == "javascript" || runtime == "elpify" || runtime == "elpian" || runtime == "fire"
}

func (wm *Vmm) resolveVmExecutionTarget(machineId string, entityId string) (string, string) {
	astPath := wm.app.Tools().Storage().StorageRoot() + "/machines/" + machineId + "/module"
	vmType := "wasm"
	wm.app.ModifyState(true, func(trx trx.ITrx) error {
		vm := model.Program{MachineId: machineId}.Pull(trx)
		if vm.Path != "" {
			astPath = vm.Path
		}
		if vm.Runtime != "" {
			vmType = normalizeRuntime(vm.Runtime)
		}
		if entityId != "" {
			if entityRuntime := trx.GetLink("vmEntityType::" + machineId + "::" + entityId); entityRuntime != "" {
				vmType = normalizeRuntime(entityRuntime)
			}
			if entityPath := trx.GetLink("vmEntityPath::" + machineId + "::" + entityId); entityPath != "" {
				astPath = entityPath
			}
		}
		return nil
	})
	return astPath, vmType
}

func (wm *Vmm) RunVm(machineId string, storeId string, data string) {
	wm.RunVmEntity(machineId, storeId, data, "")
}

func (wm *Vmm) RunVmEntity(machineId string, storeId string, data string, entityId string) {
	store := model.Store{Id: storeId}
	isMemberOfStore := false
	wm.app.ModifyState(true, func(trx trx.ITrx) error {
		store.Pull(trx)
		isMemberOfStore = (trx.GetLink("hasaccess::"+machineId+"::"+storeId) == "true")
		return nil
	})
	if !isMemberOfStore {
		return
	}
	astPath, vmType := wm.resolveVmExecutionTarget(machineId, entityId)
	b, _ := json.Marshal(updates_stores.Send{User: model.User{}, Store: store, Action: "single", Data: data})
	input := string(b)
	str, _ := json.Marshal(map[string]any{
		"type":      "runVm",
		"machineId": machineId,
		"input":     input,
		"astPath":   astPath,
		"vmType":    vmType,
	})
	wm.aeSocket <- string(str)
}

func (wm *Vmm) TerminateVm(machineId string) {
	str, _ := json.Marshal(map[string]any{
		"type":      "terminateVm",
		"machineId": machineId,
	})
	wm.aeSocket <- string(str)
}

func (wm *Vmm) BuildVmImage(machineId string, entityId string, buildPath string, buildType string) {
	str, _ := json.Marshal(map[string]any{
		"type":           "buildVmImage",
		"runtime":        buildType,
		"machineId":      machineId,
		"entityId":       entityId,
		"imageBuildPath": buildPath,
		"buildType":      buildType,
	})
	wm.aeSocket <- string(str)
}

// VmCallback is implemented in hostcall_global.go to keep host-call routing
// separated from runtime bootstrapping logic in this file.

func (wm *Vmm) handleCheckTokenValidity(input map[string]any, reqId int64) (string, int64) {
	tokenOwnerId, err := checkField(input, "tokenOwnerId", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	tokenId, err := checkField(input, "tokenId", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	gasLimit := int64(0)
	wm.app.ModifyState(true, func(trx trx.ITrx) error {
		if trx.GetString("Temp::User::"+tokenOwnerId+"::consumedTokens::"+tokenId) == "true" {
			return nil
		}
		if m, e := trx.GetJson("Json::User::"+tokenOwnerId, "lockedTokens."+tokenId); e == nil {
			gasLimit = int64(m["amount"].(float64))
		}
		return nil
	})
	jsn, _ := json.Marshal(map[string]any{"gasLimit": gasLimit})
	return string(jsn), reqId
}

func (wm *Vmm) handlePlantTrigger(input map[string]any, reqId int64) (string, int64) {
	count, err := checkField(input, "count", float64(0))
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	machineId, err := checkField(input, "machineId", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	tag, err := checkField(input, "tag", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	storeId, err := checkField(input, "storeId", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	data, err := checkField(input, "input", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	if tag == "alarm" {
		future.Async(func() {
			wm.app.ModifyState(false, func(trx trx.ITrx) error {
				trx.PutLink("vmAlarmStoreId::"+machineId, storeId)
				trx.PutLink("vmAlarmData::"+machineId, data)
				trx.PutLink("vmAlarmTime::"+machineId, fmt.Sprintf("%d", time.Now().UnixMilli()+(int64(count)*1000)))
				return nil
			})
			time.Sleep(time.Duration(count) * time.Second)
			wm.app.ModifyState(false, func(trx trx.ITrx) error {
				trx.DelKey("link::vmAlarmStoreId::" + machineId)
				trx.DelKey("link::vmAlarmData::" + machineId)
				trx.DelKey("link::vmAlarmTime::" + machineId)
				return nil
			})
			if wm.app.Tools().Security().HasAccessToStore(machineId, storeId) {
				wm.RunVm(machineId, storeId, data)
			}
		}, false)
	} else {
		wm.app.PlantChainTrigger(int(count), machineId, tag, machineId, storeId, data)
	}
	return "{}", reqId
}

func (wm *Vmm) handleSignalStore(input map[string]any, reqId int64) (string, int64) {
	machineId, err := checkField(input, "machineId", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	typAndTemp, err := checkField(input, "type", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	typ := typAndTemp
	temp := false
	if tempRaw, tempErr := checkField(input, "temp", false); tempErr == nil {
		temp = tempRaw
	} else {
		parts := strings.Split(typAndTemp, "|")
		typ = parts[0]
		if len(parts) > 1 {
			temp = parts[1] == "true"
		}
	}
	storeId, err := checkField(input, "storeId", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	userId, err := checkField(input, "userId", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	data, err := checkField(input, "data", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	wm.app.ModifyStateSecurly(false, base.NewInfo(machineId, storeId), func(s state.IState) error {
		_, _, err := wm.app.Actor().FetchAction("/stores/signal").Act(s, inputs_stores.SignalInput{
			Type:    typ,
			Data:    data,
			StoreId: storeId,
			UserId:  userId,
			Temp:    temp,
		})
		return err
	})
	return "{}", reqId
}

func parseChainReceivers(input map[string]any) map[string]map[string]bool {
	receivers := map[string]map[string]bool{}
	receiversRaw, ok := input["receivers"]
	if !ok {
		return map[string]map[string]bool{"*": map[string]bool{}}
	}
	nodesMap, ok := receiversRaw.(map[string]any)
	if !ok {
		return receivers
	}
	for nodeId, machineIdsRaw := range nodesMap {
		receivers[nodeId] = map[string]bool{}
		if machineIds, ok := machineIdsRaw.([]any); ok {
			for _, machineIdRaw := range machineIds {
				if machineId, ok := machineIdRaw.(string); ok {
					receivers[nodeId][machineId] = true
				}
			}
		}
	}
	if len(receivers) == 0 {
		receivers["*"] = map[string]bool{}
	}
	return receivers
}

func parseChainPayPacket(input map[string]any) *chain.ChainPayPacket {
	payRaw, ok := input["pay"]
	if !ok {
		return nil
	}
	payMap, ok := payRaw.(map[string]any)
	if !ok {
		return nil
	}
	pay := &chain.ChainPayPacket{}
	if v, ok := payMap["type"].(string); ok {
		pay.Type = v
	}
	if v, ok := payMap["sessionId"].(string); ok {
		pay.SessionId = v
	}
	if v, ok := payMap["userId"].(string); ok {
		pay.UserId = v
	}
	if v, ok := payMap["lockId"].(string); ok {
		pay.LockId = v
	}
	if v, ok := payMap["lockSignature"].(string); ok {
		pay.LockSignature = v
	}
	if v, ok := payMap["storeId"].(string); ok {
		pay.StoreId = v
	}
	if v, ok := payMap["vmPayload"].(string); ok {
		pay.VmPayload = v
	}
	if v, ok := payMap["error"].(string); ok {
		pay.Error = v
	}
	if v, ok := payMap["amount"].(float64); ok {
		pay.Amount = int64(v)
	}
	if v, ok := payMap["requestedSeconds"].(float64); ok {
		pay.RequestedSeconds = int64(v)
	}
	if v, ok := payMap["acceptedSeconds"].(float64); ok {
		pay.AcceptedSeconds = int64(v)
	}
	if v, ok := payMap["costPerSecond"].(float64); ok {
		pay.CostPerSecond = int64(v)
	}
	if machineIdsRaw, ok := payMap["machineIds"].([]any); ok {
		pay.MachineIds = []string{}
		for _, m := range machineIdsRaw {
			if machineId, ok := m.(string); ok {
				pay.MachineIds = append(pay.MachineIds, machineId)
			}
		}
	}
	return pay
}

func (wm *Vmm) handleSendMessageOnChain(input map[string]any, reqId int64) (string, int64) {
	chainId, _ := checkField(input, "chainId", "main")
	key, err := checkField(input, "msgKey", "")
	if err != nil {
		key, err = checkField(input, "key", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
	}
	messageType, _ := checkField(input, "messageType", "vm.execute")
	payloadStr, _ := checkField(input, "payload", "{}")
	signature, _ := checkField(input, "signature", "")
	userId, _ := checkField(input, "userId", wm.app.OwnerId())
	replyTo, _ := checkField(input, "replyTo", "")
	storeId, _ := checkField(input, "storeId", "")
	receivers := parseChainReceivers(input)
	pay := parseChainPayPacket(input)
	wm.app.Globe().SendTypedMessageOnChain(chainId, key, messageType, []byte(payloadStr), signature, userId, receivers, replyTo, storeId, pay, nil)
	return "{}", reqId
}

func (wm *Vmm) handleTerminateVM(input map[string]any, reqId int64) (string, int64) {
	targetRuntime, err := checkField(input, "runtime", "")
	if err != nil {
		return err.Error(), reqId
	}
	targetRuntime = normalizeRuntime(targetRuntime)
	targetMachineId, _ := checkField(input, "machineId", "")
	if targetRuntime == "docker" {
		vmId, _ := checkField(input, "vmId", "")
		if vmId != "" {
			entityId, _ := checkField(input, "entityId", "")
			if entityId == "" {
				entityId, _ = checkField(input, "imageName", "main")
			}
			containerName, _ := checkField(input, "containerName", "main")
			str, _ := json.Marshal(map[string]any{
				"type":          "terminateVm",
				"runtime":       "docker",
				"machineId":     targetMachineId,
				"entityId":      entityId,
				"containerName": containerName,
				"vmId":          vmId,
			})
			wm.aeSocket <- string(str)
			return "{}", reqId
		}
		return "unsupported runtime", reqId
	}
	if isManagedRuntime(targetRuntime) {
		str, _ := json.Marshal(map[string]any{
			"type":      "terminateVm",
			"runtime":   targetRuntime,
			"machineId": targetMachineId,
			"vmId":      checkVmId(input),
		})
		wm.aeSocket <- string(str)
		return "{}", reqId
	}
	return "unsupported runtime", reqId
}

func checkVmId(input map[string]any) string {
	if vmId, ok := input["vmId"].(string); ok && vmId != "" {
		return vmId
	}
	return "main"
}

func checkField[T any](input map[string]any, fieldName string, defVal T) (T, error) {
	fRaw, ok := input[fieldName]
	if !ok {
		return defVal, errors.New("{\"error\":1}}")
	}
	f, ok := fRaw.(T)
	if !ok {
		return defVal, errors.New("{\"error\":2}}")
	}
	if newF, ok := fRaw.(string); ok {
		return any(string([]byte(newF))).(T), nil
	}
	return f, nil
}

func NewVmm(core core.ICore, storageRoot string, storage storage.IStorage, kvDbPath string, file file.IFile) *Vmm {
	os.MkdirAll(kvDbPath, os.ModePerm)
	wm := &Vmm{
		app:         core,
		storageRoot: storageRoot,
		storage:     storage,
		file:        file,
		aeSocket:    make(chan string, 1000),
	}
	future.Async(func() {
		zctx, _ := zmq.NewContext()
		s, _ := zctx.NewSocket(zmq.REP)
		s.Bind("tcp://*:5555")

		zctx2, _ := zmq.NewContext()
		fmt.Printf("Connecting to the app engine server...\n")
		s2, _ := zctx2.NewSocket(zmq.REQ)
		s2.Connect("tcp://localhost:5556")

		future.Async(func() {
			for {
				msg := <-wm.aeSocket
				s2.Send(msg, 0)
				s2.Recv(0)
			}
		}, true)

		for {
			msg, _ := s.Recv(0)
			log.Printf("Received %s\n", msg)
			future.Async(func() {
				res, reqId := wm.VmCallback(msg)
				result, _ := json.Marshal(map[string]any{
					"type":      "apiResponse",
					"requestId": reqId,
					"data":      res,
				})
				wm.aeSocket <- string(result)
			}, false)
			s.Send("", 0)
		}
	}, true)
	return wm
}

func (wm *Vmm) CloseKVDB() {
	// C.close()
}
