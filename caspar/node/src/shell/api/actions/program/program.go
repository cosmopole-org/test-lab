package actions_program

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/state"
	inputs_machiner "kasper/src/shell/api/inputs/program"
	inputs_users "kasper/src/shell/api/inputs/users"
	"kasper/src/shell/api/model"
	outputs_machiner "kasper/src/shell/api/outputs/plugin"
	"kasper/src/shell/utils/future"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const pluginsTemplateName = "/machines/"

type Actions struct {
	App            core.ICore
	vmBillingLock  sync.Mutex
	lastBillingMin int64
}

func normalizeEntityType(entityType string) string {
	return strings.ToLower(strings.TrimSpace(entityType))
}

func isSupportedEntityType(entityType string) bool {
	switch normalizeEntityType(entityType) {
	case "docker", "wasm", "elpify", "javascript", "elpian", "fire":
		return true
	default:
		return false
	}
}

func entityRuntimeFileName(entityType string) string {
	switch normalizeEntityType(entityType) {
	case "elpify":
		return "module.elpify.js"
	case "javascript":
		return "module.js"
	case "elpian":
		return "module.elpian.json"
	default:
		return "module.wasm"
	}
}

func toInt64(raw any) (int64, bool) {
	switch v := raw.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

type VmResources struct {
	MaxExecTimeSeconds int64 `json:"maxExecTimeSeconds"`
	RamMb              int64 `json:"ramMb"`
	DiskGb             int64 `json:"diskGb"`
	CpuCores           int64 `json:"cpuCores"`
}

func normalizeVmResources(in inputs_machiner.VmResourcesInput) VmResources {
	res := VmResources{
		MaxExecTimeSeconds: in.MaxExecTimeSeconds,
		RamMb:              in.RamMb,
		DiskGb:             in.DiskGb,
		CpuCores:           in.CpuCores,
	}
	if res.MaxExecTimeSeconds <= 0 {
		res.MaxExecTimeSeconds = 60
	}
	if res.RamMb <= 0 {
		res.RamMb = 64
	}
	if res.DiskGb <= 0 {
		res.DiskGb = 1
	}
	if res.CpuCores <= 0 {
		res.CpuCores = 1
	}
	return res
}

func (a *Actions) vmPerMinuteCost(resources VmResources) int64 {
	cost := (resources.RamMb * a.App.VmRamCostPerMbPerMinute()) +
		(resources.CpuCores * a.App.VmCpuCoreCostPerMinute()) +
		(resources.DiskGb * a.App.VmDiskCostPerGbPerMinute())
	if cost <= 0 {
		return 1
	}
	return cost
}

func (a *Actions) validateAndBuildVmBilling(trx trx.ITrx, payerId string, lockId string, paymentSignatures []string, resources VmResources) (map[string]any, error) {
	if lockId == "" {
		return nil, errors.New("paymentLockId is required for standalone vm execution")
	}
	payment, err := trx.GetJson("Json::User::"+payerId, "lockedTokens."+lockId)
	if err != nil {
		return nil, errors.New("payment lock not found")
	}
	target, _ := payment["userId"].(string)
	if target != a.App.OwnerId() {
		return nil, errors.New("payment lock target is invalid")
	}
	stepsRaw, ok := payment["steps"].([]any)
	if !ok || len(stepsRaw) == 0 {
		return nil, errors.New("payment lock does not include steps")
	}
	if len(paymentSignatures) != len(stepsRaw) {
		return nil, errors.New("paymentSignatures count must match lock steps count")
	}
	perMinuteCost := a.vmPerMinuteCost(resources)
	stepUnlocks := make([]int64, len(stepsRaw))
	for i, rawStep := range stepsRaw {
		step, ok := rawStep.(map[string]any)
		if !ok {
			return nil, errors.New("invalid payment lock step")
		}
		stepAmount, ok := toInt64(step["amount"])
		if !ok || stepAmount != perMinuteCost {
			return nil, errors.New("payment lock step amount must match vm per-minute resource cost")
		}
		unlockAt, ok := toInt64(step["unlockAt"])
		if !ok || unlockAt <= 0 {
			return nil, errors.New("payment lock step unlockAt is invalid")
		}
		stepUnlocks[i] = unlockAt
		if i > 0 && (stepUnlocks[i]-stepUnlocks[i-1] != int64(time.Minute/time.Millisecond)) {
			return nil, errors.New("payment lock steps must be one-minute apart")
		}
		signPayload := []byte(fmt.Sprintf("%s:%d:%d:%d:%s", lockId, i, unlockAt, stepAmount, a.App.OwnerId()))
		if success, _, _ := a.App.Tools().Security().AuthWithSignature(payerId, signPayload, paymentSignatures[i]); !success {
			return nil, errors.New("payment signature verification failed")
		}
	}
	return map[string]any{
		"payerUserId":      payerId,
		"lockId":           lockId,
		"perMinuteCost":    perMinuteCost,
		"currentStep":      0,
		"stepCount":        len(stepsRaw),
		"lastChargeMinute": int64(-1),
		"signatures":       paymentSignatures,
		"resources":        resources,
	}, nil
}

func (a *Actions) chargeRunningStandaloneVmsIfNeeded() {
	a.vmBillingLock.Lock()
	defer a.vmBillingLock.Unlock()
	now := time.Now().UTC()
	currentMinute := now.Unix() / int64(time.Minute.Seconds())
	if a.lastBillingMin == currentMinute {
		return
	}
	chargeTargets := []map[string]any{}
	a.App.ModifyState(true, func(tx trx.ITrx) error {
		vmLinks, err := tx.GetLinksList("VmBilling::", -1, -1)
		if err != nil {
			return nil
		}
		for _, link := range vmLinks {
			vmId := strings.TrimPrefix(link, "VmBilling::")
			if vmId == "" || tx.GetLink("VmStatus::"+vmId) != "running" {
				continue
			}
			billing, err := tx.GetJson("Json::VmBilling::"+vmId, "payment")
			if err != nil {
				continue
			}
			nextStep, ok := toInt64(billing["currentStep"])
			if !ok {
				continue
			}
			payerId, _ := billing["payerUserId"].(string)
			lockId, _ := billing["lockId"].(string)
			perMinuteCost, _ := toInt64(billing["perMinuteCost"])
			lastChargeMinute, _ := toInt64(billing["lastChargeMinute"])
			machineId, _ := billing["machineId"].(string)
			entityId, _ := billing["entityId"].(string)
			signaturesRaw, _ := billing["signatures"].([]any)
			if payerId == "" || lockId == "" || perMinuteCost <= 0 || machineId == "" || entityId == "" {
				continue
			}
			if lastChargeMinute == currentMinute {
				continue
			}
			signatures := make([]string, len(signaturesRaw))
			for i, rawSig := range signaturesRaw {
				s, _ := rawSig.(string)
				signatures[i] = s
			}
			if int(nextStep) < 0 {
				continue
			}
			if int(nextStep) >= len(signatures) {
				if lastChargeMinute < currentMinute {
					chargeTargets = append(chargeTargets, map[string]any{
						"vmId":      vmId,
						"machineId": machineId,
						"entityId":  entityId,
						"stopOnly":  true,
					})
				}
				continue
			}
			chargeTargets = append(chargeTargets, map[string]any{
				"vmId":        vmId,
				"payerUserId": payerId,
				"lockId":      lockId,
				"step":        int(nextStep),
				"amount":      perMinuteCost,
				"signature":   signatures[nextStep],
				"machineId":   machineId,
				"entityId":    entityId,
			})
		}
		return nil
	})
	for _, target := range chargeTargets {
		if stopOnly, _ := target["stopOnly"].(bool); stopOnly {
			a.terminateStandaloneVm(target["machineId"].(string), target["entityId"].(string), target["vmId"].(string))
			a.App.ModifyState(false, func(tx trx.ITrx) error {
				vmId := target["vmId"].(string)
				tx.DelKey("link::VmStatus::" + vmId)
				tx.DelKey("link::VmInstance::" + target["machineId"].(string) + "::" + target["entityId"].(string) + "::" + vmId)
				tx.DelKey("link::VmBilling::" + vmId)
				tx.DelJson("Json::VmBilling::"+vmId, "payment")
				return nil
			})
			continue
		}
		step := target["step"].(int)
		payload, _ := json.Marshal(inputs_users.ConsumeLockInput{
			Type:      "pay",
			UserId:    target["payerUserId"].(string),
			LockId:    target["lockId"].(string),
			Signature: target["signature"].(string),
			Amount:    target["amount"].(int64),
			Step:      &step,
		})
		consumeDone := make(chan error, 1)
		a.App.Globe().SendBaseRequestOnChain("/creatures/consumeLock", payload, a.App.SignPacketAsOwner(payload), a.App.OwnerId(), "", func(_ []byte, code int, err error) {
			if err != nil || code >= 400 {
				if err == nil {
					err = errors.New("hourly vm payment consume failed")
				}
				consumeDone <- err
				return
			}
			consumeDone <- nil
		})
		err := <-consumeDone
		a.App.ModifyState(false, func(tx trx.ITrx) error {
			vmId := target["vmId"].(string)
			if err != nil {
				a.terminateStandaloneVm(target["machineId"].(string), target["entityId"].(string), vmId)
				tx.DelKey("link::VmStatus::" + vmId)
				tx.DelKey("link::VmInstance::" + target["machineId"].(string) + "::" + target["entityId"].(string) + "::" + vmId)
				tx.DelKey("link::VmBilling::" + vmId)
				tx.DelJson("Json::VmBilling::"+vmId, "payment")
			} else if billing, e := tx.GetJson("Json::VmBilling::"+vmId, "payment"); e == nil {
				billing["currentStep"] = int64(step + 1)
				billing["lastChargeMinute"] = currentMinute
				tx.PutJson("Json::VmBilling::"+vmId, "payment", billing, true)
			}
			return nil
		})
	}
	a.lastBillingMin = currentMinute
}

func (a *Actions) terminateStandaloneVm(machineId string, entityId string, vmId string) {
	a.App.ModifyState(true, func(tx trx.ITrx) error {
		entity := model.Entity{ProgramId: machineId, EntityId: entityId}.Pull(tx)
		entityType := normalizeEntityType(entity.EntityType)
		stopInput := map[string]any{
			"runtime":   entityType,
			"machineId": machineId,
			"entityId":  entityId,
			"vmId":      vmId,
		}
		if entityType == "docker" {
			stopInput["entityId"] = entity.EntityId
			stopInput["containerName"] = tx.GetLink("VmContainerName::" + machineId + "::" + entityId + "::" + vmId)
		}
		msg, _ := json.Marshal(map[string]any{
			"key":   "terminateVm",
			"input": stopInput,
		})
		a.App.Tools().Vmm().VmCallback(string(msg))
		return nil
	})
}

func Install(a *Actions, extra ...any) error {
	a.App.ModifyState(true, func(trx trx.ITrx) error {
		programs, err := model.Program{}.All(trx, -1, -1)
		if err != nil {
			panic(err)
		}
		for _, program := range programs {
			if program.Runtime == "wasm" || program.Runtime == "elpify" || program.Runtime == "javascript" || program.Runtime == "elpian" || program.Runtime == "fire" || program.Runtime == "docker" {
				a.App.Tools().Vmm().Assign(program.MachineId)
				if storeId := trx.GetLink("vmAlarmStoreId::" + program.MachineId); storeId != "" {
					future.Async(func() {
						t, _ := strconv.ParseInt(trx.GetLink("vmAlarmTime::"+program.MachineId), 10, 64)
						ct := time.Now().UnixMilli()
						if t > ct {
							time.Sleep(time.Duration(t-ct) * time.Millisecond)
						}
						data := trx.GetLink("vmAlarmData::" + program.MachineId)
						trx.DelKey("link::vmAlarmStoreId::" + program.MachineId)
						trx.DelKey("link::vmAlarmData::" + program.MachineId)
						trx.DelKey("link::vmAlarmTime::" + program.MachineId)
						if a.App.Tools().Security().HasAccessToStore(program.MachineId, storeId) {
							a.App.Tools().Vmm().RunVm(program.MachineId, storeId, data)
						}
					}, false)
				}
			}
			var storeIds []string
			prefix := "hasaccess::" + program.MachineId + "::"
			pIds, err := trx.GetLinksList(prefix, -1, -1)
			if err != nil {
				log.Println(err)
				storeIds = []string{}
			} else {
				storeIds = pIds
			}
			for _, storeId := range storeIds {
				a.App.Tools().Signaler().JoinGroup(storeId[len(prefix):], program.MachineId)
			}
		}
		return nil
	})
	future.Async(func() {
		for {
			time.Sleep(15 * time.Second)
			func() {
				defer func() {
					if err := recover(); err != nil {
						log.Println(err)
					}
				}()
				a.chargeRunningStandaloneVmsIfNeeded()
			}()
		}
	}, false)
	return nil
}

// CreateProgram /programs/create check [ true false false ] access [ true false false false POST ]
func (a *Actions) CreateProgram(state state.IState, input inputs_machiner.CreateMachineInput) (any, error) {
	trx := state.Trx()
	if !trx.HasObj("Machine", input.AppId) {
		return nil, errors.New("machine not found")
	}
	machine := model.Machine{Id: input.AppId}.Pull(trx)
	if machine.OwnerId != state.Info().UserId() {
		return nil, errors.New("you are not owner of machine")
	}
	program := model.Program{MachineId: a.App.Tools().Storage().GenId(trx, input.Origin()), AppId: machine.Id, Path: input.Path, Runtime: input.Runtime, Comment: input.Comment}
	machine.MachinesCount++
	machine.Push(trx)
	program.Push(trx)
	trx.PutJson("ProgMeta::"+program.MachineId, "metadata", map[string]any{}, true)
	trx.PutLink("machinePrograms::"+machine.Id+"::"+program.MachineId, "true")
	return map[string]any{"program": program}, nil
}

// DeleteProgram /programs/delete check [ true false false ] access [ true false false false POST ]
func (a *Actions) DeleteProgram(state state.IState, input inputs_machiner.DeleteProgramInput) (any, error) {
	trx := state.Trx()
	if !trx.HasObj("Program", input.ProgramId) {
		return nil, errors.New("program does not exist")
	}
	appId := trx.GetIndex("Program", "id", "programId", input.ProgramId)
	machine := model.Machine{Id: appId}.Pull(trx)
	machine.MachinesCount--
	machine.Push(trx)
	trx.DelIndex("Program", "id", "programId", input.ProgramId)
	trx.DelKey("link::machinePrograms::" + machine.Id + "::" + input.ProgramId)
	return map[string]any{}, nil
}

// UpdateProgram /programs/update check [ true false false ] access [ true false false false POST ]
func (a *Actions) UpdateProgram(state state.IState, input inputs_machiner.UpdateProgramInput) (any, error) {
	trx := state.Trx()
	if !trx.HasObj("Program", input.ProgramId) {
		return nil, errors.New("program does not exist")
	}
	program := model.Program{MachineId: input.ProgramId}.Pull(trx)
	program.Path = input.Path
	program.Push(trx)
	if input.Metadata != nil {
		trx.PutJson("ProgMeta::"+program.MachineId, "metadata", input.Metadata, true)
	}
	return map[string]any{}, nil
}

// RunProgramEntity /programs/runEntity check [ true false false ] access [ true false false false POST ]
func (a *Actions) RunProgramEntity(state state.IState, input inputs_machiner.RunProgramEntityInput) (any, error) {
	trx := state.Trx()
	programId := input.ProgramId
	if programId == "" {
		programId = input.MachineId
	}
	if !trx.HasObj("Program", programId) {
		return nil, errors.New("program does not exist")
	}
	program := model.Program{MachineId: programId}.Pull(trx)
	entity := model.Entity{ProgramId: program.MachineId, EntityId: input.EntityId}.Pull(trx)
	if entity.EntityId == "" {
		return nil, errors.New("entity does not exist")
	}
	entityType := normalizeEntityType(entity.EntityType)
	machine := model.Machine{Id: program.MachineId}.Pull(trx)
	if machine.OwnerId != state.Info().UserId() {
		return nil, errors.New("you are not owner of this machine")
	}
	vmId := uuid.NewString()
	trx.PutLink("VmStatus::"+vmId, "running")
	resources := normalizeVmResources(input.Resources)
	billingData, billingErr := a.validateAndBuildVmBilling(trx, state.Info().UserId(), input.PaymentLockId, input.PaymentSignatures, resources)
	if billingErr != nil {
		return nil, billingErr
	}
	billingData["machineId"] = input.MachineId
	billingData["entityId"] = input.EntityId
	billingData["vmId"] = vmId
	trx.PutLink("VmBilling::"+vmId, "true")
	trx.PutJson("Json::VmBilling::"+vmId, "payment", billingData, true)
	params := input.Params
	if params == nil {
		params = map[string]string{}
	}
	trx.PutLink("VmInstance::"+program.MachineId+"::"+input.EntityId+"::"+vmId, "true")
	if entityType == "docker" {
		entityImageId := input.EntityId
		containerName := uuid.NewString()
		trx.PutLink("VmContainerName::"+program.MachineId+"::"+input.EntityId+"::"+vmId, containerName)
		future.Async(func() {
			msg, _ := json.Marshal(map[string]any{
				"key": "runVm",
				"input": map[string]any{
					"runtime":       entityType,
					"machineId":     input.MachineId,
					"entityId":      input.EntityId,
					"containerName": containerName,
					"standalone":    true,
					"vmId":          vmId,
					"resources":     resources,
					"imageRef":      strings.ReplaceAll(input.MachineId, "@", "_") + "/" + entityImageId,
					"inputFiles":    params,
				},
			})
			a.App.Tools().Vmm().VmCallback(string(msg))
		}, false)
		return map[string]any{"vmId": vmId}, nil
	}
	if !isSupportedEntityType(entityType) || entityType == "docker" {
		return nil, errors.New("invalid entity type")
	}
	data, _ := json.Marshal(params)
	future.Async(func() {
		msg, _ := json.Marshal(map[string]any{
			"key": "runVm",
			"input": map[string]any{
				"runtime":    entityType,
				"machineId":  input.MachineId,
				"entityId":   input.EntityId,
				"standalone": true,
				"vmId":       vmId,
				"resources":  resources,
				"data":       string(data),
			},
		})
		a.App.Tools().Vmm().VmCallback(string(msg))
	}, false)
	return map[string]any{"vmId": vmId}, nil
}

// StopProgramEntity /programs/stopEntity check [ true false false ] access [ true false false false POST ]
func (a *Actions) StopProgramEntity(state state.IState, input inputs_machiner.RunProgramEntityInput) (any, error) {
	trx := state.Trx()
	programId := input.ProgramId
	if programId == "" {
		programId = input.MachineId
	}
	if !trx.HasObj("Program", programId) {
		return nil, errors.New("program does not exist")
	}
	program := model.Program{MachineId: programId}.Pull(trx)
	entity := model.Entity{ProgramId: program.MachineId, EntityId: input.EntityId}.Pull(trx)
	if entity.EntityId == "" {
		return nil, errors.New("entity does not exist")
	}
	entityType := normalizeEntityType(entity.EntityType)
	machine := model.Machine{Id: program.MachineId}.Pull(trx)
	if machine.OwnerId != state.Info().UserId() {
		return nil, errors.New("you are not owner of this machine")
	}
	vmId := input.VmId
	trx.DelKey("link::VmStatus::" + vmId)
	trx.DelKey("link::VmInstance::" + program.MachineId + "::" + input.EntityId + "::" + vmId)
	trx.DelKey("link::VmBilling::" + vmId)
	trx.DelJson("Json::VmBilling::"+vmId, "payment")
	entityImageId := entity.EntityId
	containerName := trx.GetLink("VmContainerName::" + program.MachineId + "::" + input.EntityId + "::" + vmId)
	trx.DelKey("link::vmStandaloneImageName::" + program.MachineId + "::" + input.EntityId)
	trx.DelKey("link::vmStandaloneContainerName::" + program.MachineId + "::" + input.EntityId)
	stopInput := map[string]any{
		"runtime":   entityType,
		"machineId": input.MachineId,
		"entityId":  input.EntityId,
		"vmId":      vmId,
	}
	if entityType == "docker" {
		if entityImageId == "" || containerName == "" {
			return nil, errors.New("entity runtime links are not found")
		}
		stopInput["entityId"] = entityImageId
		stopInput["containerName"] = containerName
	}
	msg, _ := json.Marshal(map[string]any{
		"key":   "terminateVm",
		"input": stopInput,
	})
	a.App.Tools().Vmm().VmCallback(string(msg))
	return map[string]any{}, nil
}

// ReadVmLogs /machines/readVmLogs check [ true false false ] access [ true false false false POST ]
func (a *Actions) ReadVmLogs(state state.IState, input inputs_machiner.ReadVmLogsInput) (any, error) {
	ownerUserId := ""
	links, err := state.Trx().GetLinksList("VmInstance::", -1, -1)
	if err == nil {
		suffix := "::" + input.VmId
		for _, link := range links {
			if !strings.HasSuffix(link, suffix) {
				continue
			}
			parts := strings.Split(link, "::")
			if len(parts) < 4 {
				continue
			}
			program := model.Program{MachineId: parts[1]}.Pull(state.Trx())
			if program.MachineId == "" {
				break
			}
			machine := model.Machine{Id: program.AppId}.Pull(state.Trx())
			ownerUserId = machine.OwnerId
			break
		}
	}
	if ownerUserId == "" {
		return nil, errors.New("vm not found")
	}
	if ownerUserId != "" && ownerUserId != state.Info().UserId() {
		return nil, errors.New("you are not owner of this vm")
	}
	count := input.Count
	if count <= 0 {
		count = 100
	}
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}
	return map[string]any{
		"logs": a.App.Tools().Storage().ReadVmLogs(input.VmId, input.LogType, offset, count),
	}, nil
}

// OpenVmTerminal /machines/openVmTerminal check [ true false false ] access [ true false false false POST ]
func (a *Actions) OpenVmTerminal(state state.IState, input inputs_machiner.VmTerminalInput) (any, error) {
	trx := state.Trx()
	if !trx.HasObj("Program", input.CreatureId) {
		return nil, errors.New("program does not exist")
	}
	program := model.Program{MachineId: input.CreatureId}.Pull(trx)
	machine := model.Machine{Id: program.AppId}.Pull(trx)
	if machine.OwnerId != state.Info().UserId() {
		return nil, errors.New("you are not owner of this creature")
	}
	trx.PutLink("VmTerminal::"+input.CreatureId+"::"+input.VmId+"::"+state.Info().UserId(), "true")
	return map[string]any{"terminal": "on"}, nil
}

// CloseVmTerminal /machines/closeVmTerminal check [ true false false ] access [ true false false false POST ]
func (a *Actions) CloseVmTerminal(state state.IState, input inputs_machiner.VmTerminalInput) (any, error) {
	trx := state.Trx()
	if !trx.HasObj("Program", input.CreatureId) {
		return nil, errors.New("program does not exist")
	}
	program := model.Program{MachineId: input.CreatureId}.Pull(trx)
	machine := model.Machine{Id: program.AppId}.Pull(trx)
	if machine.OwnerId != state.Info().UserId() {
		return nil, errors.New("you are not owner of this creature")
	}
	trx.DelKey("link::VmTerminal::" + input.CreatureId + "::" + input.VmId + "::" + state.Info().UserId())
	return map[string]any{"terminal": "off"}, nil
}

// ReadMachineBuilds /machines/readMachineBuilds check [ true false false ] access [ true false false false POST ]
func (a *Actions) ReadMachineBuilds(state state.IState, input inputs_machiner.MachineBuildsInput) (any, error) {
	prefix := "VmBuilds::" + input.MachineId + "::"
	builds, err := state.Trx().GetLinksList(prefix, input.Offset, input.Count, false)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return map[string]any{"buildsList": builds}, nil
}

// Deploy /programs/deploy check [ true false false ] access [ true false false false POST ]
func (a *Actions) Deploy(state state.IState, input inputs_machiner.DeployInput) (any, error) {
	trx := state.Trx()
	programId := input.MachineId
	if !trx.HasObj("Program", programId) {
		return nil, errors.New("program not found")
	}
	program := model.Program{MachineId: programId}.Pull(trx)
	if !trx.HasObj("Machine", program.AppId) {
		return nil, errors.New("machine not found")
	}
	machine := model.Machine{Id: program.AppId}.Pull(trx)
	if machine.OwnerId != state.Info().UserId() {
		return nil, errors.New("access to vm denied")
	}
	entityType := normalizeEntityType(input.EntityType)
	if !isSupportedEntityType(entityType) {
		return nil, errors.New("invalid entityType, expected one of docker|wasm|elpify|javascript|elpian|fire")
	}
	data, err := base64.StdEncoding.DecodeString(input.Payload)
	if err != nil {
		return nil, err
	}
	entityModel := model.Entity{
		ProgramId:  program.MachineId,
		EntityId:   input.EntityId,
		EntityType: entityType,
		ImageName:  input.EntityId,
	}
	vmId := uuid.NewString()
	if entityType == "docker" || entityType == "wasm" {
		files := map[string]any{}
		if input.Metadata != nil {
			filesRaw, ok := input.Metadata["files"]
			if ok {
				filesCast, ok2 := filesRaw.(map[string]any)
				if !ok2 {
					return nil, errors.New("files is not map")
				}
				files = filesCast
			}
		}
		buildFolderPath := fmt.Sprintf("%s%s%s/entities/%s", a.App.Tools().Storage().StorageRoot(), pluginsTemplateName, program.MachineId, input.EntityId)
		// For wasm runtimes the payload IS the compiled artifact (no server-side
		// build step). Save it as module.wasm and point the per-entity ast path
		// link at it so resolveVmExecutionTarget can load the right binary.
		// For docker the payload is a Dockerfile that the server-side
		// BuildVmImage step compiles into an image, alongside any source files.
		var primaryFileName string
		if entityType == "wasm" {
			primaryFileName = entityRuntimeFileName(entityType) // module.wasm
		} else {
			primaryFileName = "Dockerfile"
		}
		err2 := a.App.Tools().File().SaveDataToGlobalStorage(buildFolderPath, data, primaryFileName, true)
		if err2 != nil {
			return nil, err2
		}
		for k, v := range files {
			dataStr, ok := v.(string)
			if !ok {
				err := errors.New("file bytecode not string")
				log.Println(err)
				return nil, err
			}
			data, err := base64.StdEncoding.DecodeString(dataStr)
			if err != nil {
				return nil, err
			}
			err2 := a.App.Tools().File().SaveDataToGlobalStorage(buildFolderPath, data, k, true)
			if err2 != nil {
				return nil, err2
			}
		}
		if entityType == "wasm" {
			// Persist where the wasm lives, so the VMM resolves to this artifact
			// when signals arrive (resolveVmExecutionTarget reads this link).
			trx.PutLink("vmEntityPath::"+program.MachineId+"::"+input.EntityId, buildFolderPath+"/"+primaryFileName)
			trx.PutLink("vmEntityType::"+program.MachineId+"::"+input.EntityId, "wasm")
			program.Push(trx)
			a.App.Tools().Vmm().Assign(program.MachineId)
		} else {
			buildId := uuid.NewString()
			trx.PutLink("VmBuilds::"+vmId+"::"+buildId, "true")
			future.Async(func() {
				a.App.Tools().Vmm().BuildVmImage(program.MachineId, input.EntityId, buildFolderPath, entityType)
			}, false)
		}
	} else {
		fileName := entityRuntimeFileName(entityType)
		entityFolderPath := fmt.Sprintf("%s%s%s/entities/%s", a.App.Tools().Storage().StorageRoot(), pluginsTemplateName, program.MachineId, input.EntityId)
		err2 := a.App.Tools().File().SaveDataToGlobalStorage(entityFolderPath, data, fileName, true)
		if err2 != nil {
			return nil, err2
		}
		if entityType == "elpify" {
			buildId := uuid.NewString()
			trx.PutLink("VmBuilds::"+vmId+"::"+buildId, "true")
			future.Async(func() {
				a.App.Tools().Vmm().BuildVmImage(program.MachineId, input.EntityId, entityFolderPath, entityType)
			}, false)
		}
		program.Push(trx)
		a.App.Tools().Vmm().Assign(program.MachineId)
	}
	entityModel.EntityType = entityType
	entityModel.Push(trx)
	return outputs_machiner.PlugInput{}, nil
}

// ListMachines /machines/list check [ true false false ] access [ true false false false GET ]
func (a *Actions) ListMachines(state state.IState, input inputs_machiner.ListInput) (any, error) {
	trx := state.Trx()
	machines, err := model.Machine{}.All(trx, input.Offset, input.Count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	result := []map[string]any{}
	for _, machine := range machines {
		profile, err := trx.GetJson("CreatMeta::"+machine.Id, "metadata.public.profile")
		if err != nil {
			log.Println(err)
			result = append(result, map[string]any{
				"id":            machine.Id,
				"chainId":       machine.ChainId,
				"username":      machine.Username,
				"ownerId":       machine.OwnerId,
				"programsCount": machine.MachinesCount,
				"title":         "untitled",
				"avatar":        "",
				"desc":          "",
			})
			continue
		}
		result = append(result, map[string]any{
			"id":            machine.Id,
			"chainId":       machine.ChainId,
			"username":      machine.Username,
			"ownerId":       machine.OwnerId,
			"programsCount": machine.MachinesCount,
			"title":         profile["title"],
			"avatar":        profile["avatar"],
			"desc":          profile["desc"],
		})
	}
	return map[string]any{"machines": result}, nil
}

// ListPrograms /programs/list check [ true false false ] access [ true false false false GET ]
func (a *Actions) ListPrograms(state state.IState, input inputs_machiner.ListInput) (any, error) {
	trx := state.Trx()
	machines, err := model.Program{}.All(trx, input.Offset, input.Count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return map[string]any{"machines": machines}, nil
}

// ListProgramMachines /machines/listProgramMachines check [ true false false ] access [ true false false false GET ]
func (a *Actions) ListProgramMachines(state state.IState, input inputs_machiner.ListAppMachsInput) (any, error) {
	trx := state.Trx()
	machines, err := model.User{}.List(trx, "machinePrograms::"+input.AppId+"::", map[string]string{})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	programs, err := model.Program{}.List(trx, "machinePrograms::"+input.AppId+"::")
	if err != nil {
		log.Println(err)
		return nil, err
	}
	programByMachineID := map[string]model.Program{}
	for _, program := range programs {
		programByMachineID[program.MachineId] = program
	}
	result := []map[string]any{}
	for _, machine := range machines {
		result = append(result, map[string]any{
			"id":       machine.Id,
			"type":     machine.Typ,
			"username": machine.Username,
			"comment":  programByMachineID[machine.Id].Comment,
		})
	}
	return map[string]any{"machines": result}, nil
}

