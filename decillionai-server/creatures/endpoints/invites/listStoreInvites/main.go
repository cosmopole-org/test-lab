package main

import (
	"encoding/json"
	"runtime"
	"strings"
	"unsafe"
)

//go:wasmimport env hostCall
func hostCall(offset uint32, length uint32) uint64

type packet struct {
	Payload    map[string]any `json:"payload"`
	CreatureID string         `json:"creatureId,omitempty"`
	StoreID    string         `json:"storeId,omitempty"`
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

func parseHostJSON(raw string) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &m)
	return m
}

func parsePrefixRows(raw string) []map[string]any {
	rows := []map[string]any{}
	if raw == "" {
		return rows
	}
	parsed := any(nil)
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return rows
	}
	switch v := parsed.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				rows = append(rows, m)
			}
		}
	case map[string]any:
		if nested, ok := v["items"].([]any); ok {
			for _, item := range nested {
				if m, ok := item.(map[string]any); ok {
					rows = append(rows, m)
				}
			}
		} else {
			for k, vv := range v {
				rows = append(rows, map[string]any{"key": k, "val": vv})
			}
		}
	}
	return rows
}

func extractString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func process(input string) string {
	p := packet{}
	if input != "" {
		_ = json.Unmarshal([]byte(input), &p)
	}
	setHostContext(p.CreatureID, p.Payload)
	targetCreatureID, _ := p.Payload["targetCreatureId"].(string)
	if targetCreatureID == "" {
		targetCreatureID, _ = p.Payload["machineId"].(string)
	}
	hostReq("putJson", map[string]any{
		"key":   "Json::CreatureEndpoint::invites::listStoreInvites",
		"path":  "lastInput",
		"data":  p.Payload,
		"merge": true,
	})
	packetBytes, _ := json.Marshal(map[string]any{"endpoint": "/invites/listStoreInvites", "payload": p.Payload})
	signalKey := "invites/listStoreInvites"
	if p.CreatureID != "" {
		hostReq("dbOp", map[string]any{"op": "put", "key": "creatureEndpoint::invites::listStoreInvites::lastCreature", "val": p.CreatureID})
	}
	if p.StoreID != "" {
		hostReq("signalGroup", map[string]any{"key": signalKey, "groupId": p.StoreID, "packet": string(packetBytes), "system": true})
	}
	if p.CreatureID != "" {
		hostReq("signalUser", map[string]any{"key": signalKey, "userId": p.CreatureID, "creatureId": p.CreatureID, "packet": string(packetBytes), "system": true})
	}
	if targetCreatureID != "" && p.StoreID != "" {
		hostReq("hasAccessToStore", map[string]any{"machineId": targetCreatureID, "targetCreatureId": targetCreatureID, "storeId": p.StoreID})
	}
	storeID := p.StoreID
	if sid, _ := p.Payload["storeId"].(string); sid != "" {
		storeID = sid
	}
	users := []map[string]any{}
	if storeID != "" {
		prefix := "link::invite::" + storeID + "::"
		rows := parsePrefixRows(hostReq("dbOp", map[string]any{"op": "getByPrefix", "prefix": prefix}))
		for _, row := range rows {
			key := extractString(row, "key", "k")
			if key == "" {
				continue
			}
			userID := strings.TrimPrefix(key, prefix)
			if userID == "" || userID == key {
				continue
			}
			timeRaw := hostReq("dbOp", map[string]any{"op": "get", "key": "link::invitetime::" + storeID + "::" + userID})
			timeVal := extractString(parseHostJSON(timeRaw), "val", "value", "data")
			users = append(users, map[string]any{"id": userID, "time": timeVal})
		}
	}
	out, _ := json.Marshal(map[string]any{"users": users})
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
