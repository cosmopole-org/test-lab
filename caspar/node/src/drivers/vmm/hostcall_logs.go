package vmm

import (
	"kasper/src/abstract/models/trx"
	"kasper/src/shell/api/model"
	"strings"
	"time"
)

func (wm *Vmm) resolveVmOwnership(vmId string) (string, string) {
	if vmId == "" {
		return "", ""
	}
	creatureId := ""
	ownerUserId := ""
	wm.app.ModifyState(true, func(tx trx.ITrx) error {
		links, err := tx.GetLinksList("VmInstance::", -1, -1)
		if err != nil {
			return nil
		}
		suffix := "::" + vmId
		for _, link := range links {
			if !strings.HasSuffix(link, suffix) {
				continue
			}
			parts := strings.Split(link, "::")
			if len(parts) < 4 {
				continue
			}
			creatureId = parts[1]
			program := model.Program{MachineId: creatureId}.Pull(tx)
			if program.MachineId == "" {
				break
			}
			machine := model.Machine{Id: program.AppId}.Pull(tx)
			ownerUserId = machine.OwnerId
			break
		}
		return nil
	})
	return creatureId, ownerUserId
}

func (wm *Vmm) handleVmLogEvent(input map[string]any, reqId int64) (string, int64) {
	text, _ := checkField(input, "text", "")
	data := text
	if data == "" {
		data, _ = checkField(input, "data", "")
	}
	if data == "" {
		return "{}", reqId
	}
	vmId, _ := checkField(input, "vmId", "")
	logType, _ := checkField(input, "logType", "")
	if logType == "" {
		logType = "runtime"
	}
	timeVal := time.Now().UnixMilli()
	if vmId == "" {
		return "{}", reqId
	}
	creatureId, ownerUserId := wm.resolveVmOwnership(vmId)
	packet := wm.storage.LogVm(vmId, logType, data, timeVal)
	if ownerUserId != "" && vmId != "" {
		terminalOn := false
		wm.app.ModifyState(true, func(tx trx.ITrx) error {
			terminalOn = tx.GetLink("VmTerminal::"+creatureId+"::"+vmId+"::"+ownerUserId) == "true"
			return nil
		})
		if terminalOn {
			wm.app.Tools().Signaler().SignalUser("machines/vmLogs", ownerUserId, map[string]any{
				"log": packet,
			}, true)
		}
	}
	if strings.TrimSpace(data) == "" {
		return "{}", reqId
	}
	return "{}", reqId
}
