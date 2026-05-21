package vmm

import "encoding/json"

// VmCallback handles appengine host-call packets globally for all appengine
// runtimes (wasm/docker/javascript/elpify/elpian/fire) through the same ZeroMQ callback channel.
func (wm *Vmm) VmCallback(dataRaw string) (string, int64) {
	println(dataRaw)
	data := map[string]any{}
	err := json.Unmarshal([]byte(dataRaw), &data)
	if err != nil {
		println(err)
		return err.Error(), 0
	}
	reqIdRaw, err := checkField(data, "requestId", float64(0))
	if err != nil {
		println(err)
		return err.Error(), 0
	}
	reqId := int64(reqIdRaw)
	key, err := checkField(data, "key", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	input, err := checkField[map[string]any](data, "input", nil)
	if err != nil {
		println(err)
		return err.Error(), reqId
	}

	switch key {
	case "execDocker", "copyToDocker":
		return "unsupported runtime", reqId
	case "execVm", "copyToVm":
		return "unsupported runtime", reqId
	case "checkTokenValidity":
		return wm.handleCheckTokenValidity(input, reqId)
	case "plantTrigger":
		return wm.handlePlantTrigger(input, reqId)
	case "signal":
		return wm.handleSignalStore(input, reqId)
	case "runVm":
		return "unsupported runtime", reqId
	case "terminateVm":
		return wm.handleTerminateVM(input, reqId)
	case "sendMessageOnChain":
		return wm.handleSendMessageOnChain(input, reqId)
	case "createCreature":
		return wm.handleCreatureCrud("create", input, reqId)
	case "updateCreature":
		return wm.handleCreatureCrud("update", input, reqId)
	case "deleteCreature":
		return wm.handleCreatureCrud("delete", input, reqId)
	case "getCreature":
		return wm.handleCreatureCrud("get", input, reqId)
	case "listCreatures":
		return wm.handleCreatureCrud("list", input, reqId)
	case "createResourceStore", "createVmOwnedStore":
		return wm.handleResourceStoreCrud("create", input, reqId)
	case "updateResourceStore", "updateVmOwnedStore":
		return wm.handleResourceStoreCrud("update", input, reqId)
	case "deleteResourceStore", "deleteVmOwnedStore":
		return wm.handleResourceStoreCrud("delete", input, reqId)
	case "getResourceStore", "getVmOwnedStore":
		return wm.handleResourceStoreCrud("get", input, reqId)
	case "listResourceStores", "listVmOwnedStores":
		return wm.handleResourceStoreCrud("list", input, reqId)
	case "createStore":
		return wm.handleStoreCrud("create", input, reqId)
	case "updateStore":
		return wm.handleStoreCrud("update", input, reqId)
	case "deleteStore":
		return wm.handleStoreCrud("delete", input, reqId)
	case "getStore":
		return wm.handleStoreCrud("get", input, reqId)
	case "listStores":
		return wm.handleStoreCrud("list", input, reqId)
	case "createResourceEntity":
		return wm.handleResourceEntityCreate(input, reqId)
	case "deleteResourceEntity":
		return wm.handleResourceEntityDelete(input, reqId)
	case "createWorkchain":
		return wm.handleVmChainRequest("createWorkchain", input, reqId)
	case "deleteWorkchain":
		return wm.handleVmChainRequest("deleteWorkchain", input, reqId)
	case "createSubchain":
		return wm.handleVmChainRequest("createSubchain", input, reqId)
	case "deleteSubchain":
		return wm.handleVmChainRequest("deleteSubchain", input, reqId)
	case "execShellAction":
		return wm.handleExecShellAction(input, reqId)
	case "genId":
		return wm.handleMicroHostAction("genId", input, reqId)
	case "getLink":
		return wm.handleMicroHostAction("getLink", input, reqId)
	case "delKey":
		return wm.handleMicroHostAction("delKey", input, reqId)
	case "createAccess":
		return wm.handleMicroHostAction("createAccess", input, reqId)
	case "deleteAccess":
		return wm.handleMicroHostAction("deleteAccess", input, reqId)
	case "getJson":
		return wm.handleMicroHostAction("getJson", input, reqId)
	case "putJson":
		return wm.handleMicroHostAction("putJson", input, reqId)
	case "getByPrefix":
		return wm.handleMicroHostAction("getByPrefix", input, reqId)
	case "hasAccessToStore":
		return wm.handleMicroHostAction("hasAccessToStore", input, reqId)
	case "signalUser":
		return wm.handleMicroHostAction("signalUser", input, reqId)
	case "signalGroup":
		return wm.handleMicroHostAction("signalGroup", input, reqId)
	case "joinGroup":
		return wm.handleMicroHostAction("joinGroup", input, reqId)
	case "log":
		return wm.handleVmLogEvent(input, reqId)
	case "vmLog", "buildLog":
		return wm.handleVmLogEvent(input, reqId)
	case "output", "vmOutput":
		// Creatures emit their final JSON response via the `output` host op.
		// appengine captures it locally as `execution_result` and re-emits it
		// after wasm finalize as `vmOutput` so we have a single place to
		// route the data back to whoever triggered the run. For now, persist
		// it as a runtime log keyed by machine/vm so downstream tooling and
		// audit pipelines can read it.
		return wm.handleVmLogEvent(input, reqId)
	}

	return "{}", reqId
}
