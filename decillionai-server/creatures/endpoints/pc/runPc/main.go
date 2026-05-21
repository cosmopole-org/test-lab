package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"unsafe"
)

//go:wasmimport env hostCall
func hostCall(offset uint32, length uint32) uint64

type packet struct {
	Payload          map[string]any `json:"payload"`
	CreatureID       string         `json:"creatureId,omitempty"`
	StoreID          string         `json:"storeId,omitempty"`
	TargetCreatureID string         `json:"targetCreatureId,omitempty"`
}

func bytesAt(offset uint32, length uint32) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(offset))), length)
}

func stringAt(offset uint32, length uint32) string {
	if length == 0 {
		return ""
	}
	return string(bytesAt(offset, length))
}

func bytesToPointer(data []byte) (uint32, uint32) {
	if len(data) == 0 {
		return 0, 0
	}
	return uint32(uintptr(unsafe.Pointer(&data[0]))), uint32(len(data))
}

func decodeHostResult(ret uint64) (uint32, uint32) {
	return uint32(ret >> 32), uint32(ret)
}

func hostRequest(req string) string {
	reqBytes := []byte(req)
	ptr, length := bytesToPointer(reqBytes)
	ret := hostCall(ptr, length)
	runtime.KeepAlive(reqBytes)
	retOffset, retLen := decodeHostResult(ret)
	return stringAt(retOffset, retLen)
}

var hostCreatureID string
var hostProgramID string
var hostEntityName string
var hostEntityPath string

func extractContextString(input map[string]any, keys ...string) string {
	if input == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := input[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func setHostContext(creatureID string, payload map[string]any) {
	hostCreatureID = creatureID
	if hostCreatureID == "" {
		hostCreatureID = extractContextString(payload, "creatureId", "userId")
	}

	hostProgramID = extractContextString(payload, "programId", "targetCreatureId", "machineId")
	if hostProgramID == "" {
		hostProgramID = hostCreatureID
	}

	hostEntityName = extractContextString(payload, "entityId", "entityName", "entity", "name")
	hostEntityPath = extractContextString(payload, "entityPath", "astPath", "astpath", "path")
}

func hostReq(op string, input map[string]any) string {
	creatureID := hostCreatureID
	programID := hostProgramID
	entityName := hostEntityName
	entityPath := hostEntityPath
	if input != nil {
		if v := extractContextString(input, "creatureId", "userId"); v != "" {
			creatureID = v
		}
		if v := extractContextString(input, "programId", "targetCreatureId", "machineId"); v != "" {
			programID = v
		}
		if v := extractContextString(input, "entityId", "entityName", "entity", "name"); v != "" {
			entityName = v
		}
		if v := extractContextString(input, "entityPath", "astPath", "astpath", "path"); v != "" {
			entityPath = v
		}
	}

	if programID == "" {
		programID = "system"
	}

	req := map[string]any{
		"creatureId": creatureID,
		"programId":  programID,
		"entityId":   entityName,
		"entityPath": entityPath,
		"op":         op,
		"input":      input,
	}
	b, _ := json.Marshal(req)
	return hostRequest(string(b))
}

func process(input string) string {
	p := packet{}
	if input != "" {
		_ = json.Unmarshal([]byte(input), &p)
	}
	setHostContext(p.CreatureID, p.Payload)
	vmID, _ := p.Payload["vmId"].(string)

	// Forward streamed firecracker output events to the original requester.
	if p.Payload["event"] == "fireVmOutput" {
		requesterCreatureID, _ := p.Payload["requesterCreatureId"].(string)
		if requesterCreatureID == "" {
			requesterCreatureID, _ = p.Payload["requesterUserId"].(string)
		}
		if requesterCreatureID != "" {
			streamPayload, _ := json.Marshal(map[string]any{
				"type":   "pc.fire.stream",
				"source": "pc.runPc",
				"data":   p.Payload,
			})
			hostReq("signalUser", map[string]any{
				"key":        "pc/message",
				"userId":     requesterCreatureID,
				"creatureId": requesterCreatureID,
				"packet":     string(streamPayload),
				"system":     true,
			})
		}
		out, _ := json.Marshal(map[string]any{"vmId": vmID})
		return string(out)
	}

	if vmID == "" {
		genRes := hostReq("genId", map[string]any{"source": "pc.runPc.vm"})
		genMap := map[string]any{}
		_ = json.Unmarshal([]byte(genRes), &genMap)
		vmID, _ = genMap["id"].(string)
		if vmID == "" {
			vmID = "main"
		}
	}

	command := ""
	if v, ok := p.Payload["command"].(string); ok {
		command = strings.TrimSpace(v)
	}
	runInput := map[string]any{
		"runtime":             "fire",
		"machineId":           p.TargetCreatureID,
		"targetCreatureId":    p.TargetCreatureID,
		"storeId":             p.StoreID,
		"requesterUserId":     p.CreatureID,
		"requesterCreatureId": p.CreatureID,
		"vmId":                vmID,
		"standalone":          true,
	}
	if command != "" {
		runInput["data"] = fmt.Sprintf("{\"command\":%q}", command)
	}
	hostReq("runVm", runInput)

	hostReq("putJson", map[string]any{
		"key":   "Json::CreatureEndpoint::pc::runPc",
		"path":  "lastFireRun",
		"data":  map[string]any{"machineId": p.TargetCreatureID, "targetCreatureId": p.TargetCreatureID, "storeId": p.StoreID, "requesterUserId": p.CreatureID, "requesterCreatureId": p.CreatureID, "vmId": vmID, "command": command},
		"merge": true,
	})

	out, _ := json.Marshal(map[string]any{"vmId": vmID})
	hostReq("output", map[string]any{"text": string(out)})
	return string(out)
}

//export run
func run(arg uint64) int64 {
	input := stringAt(uint32(arg>>32), uint32(arg))
	process(input)
	return 0
}

func main() {}
