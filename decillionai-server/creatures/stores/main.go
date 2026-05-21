package main

import (
	"encoding/json"
	"runtime"
	"unsafe"
)

//go:wasmimport env hostCall
func hostCall(offset uint32, length uint32) uint64

type packet struct {
	Path          string         `json:"path"`
	Payload       map[string]any `json:"payload"`
	CreatureID    string         `json:"creatureId,omitempty"`
	SpaceID       string         `json:"spaceId,omitempty"`
	RequesterID   string         `json:"requesterId,omitempty"`
	CorrelationID string         `json:"correlationId,omitempty"`
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

// unwrapSignal extracts the user-supplied {path, payload, ...} from the
// framework's runVm input envelope. The framework wraps signals as
//
//	{ "user": {...}, "store": {...}, "action": "single",
//	  "data": "<json>", "entityId": "..." }
//
// where data is a JSON string of { programId, entity, payload }, and payload
// itself is another JSON string of { action, programId, entity, payload }.
// We peel both layers so creature code can dispatch on action/path uniformly.
func unwrapSignal(input string) packet {
	p := packet{}
	if input == "" {
		return p
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		// Fall back to the legacy shape (already a packet).
		_ = json.Unmarshal([]byte(input), &p)
		return p
	}
	// Direct legacy shape: {path, payload, creatureId, spaceId}
	if _, hasPath := raw["path"]; hasPath {
		_ = json.Unmarshal([]byte(input), &p)
		return p
	}
	if user, ok := raw["user"].(map[string]any); ok {
		if id, ok := user["id"].(string); ok {
			// The framework calls this "user" — for our purposes the user is
			// also the requester whose CLI sent the signal.
			p.CreatureID = id
			p.RequesterID = id
		}
	}
	if store, ok := raw["store"].(map[string]any); ok {
		if id, ok := store["id"].(string); ok {
			p.SpaceID = id
		}
	}
	// `data` is a JSON string holding
	//   { programId, entity, correlationId?, payload: "<json>" }.
	dataStr, _ := raw["data"].(string)
	if dataStr == "" {
		return p
	}
	var layer1 map[string]any
	if err := json.Unmarshal([]byte(dataStr), &layer1); err != nil {
		return p
	}
	if cid, ok := layer1["correlationId"].(string); ok {
		p.CorrelationID = cid
	}
	payloadStr, _ := layer1["payload"].(string)
	if payloadStr == "" {
		return p
	}
	var inner map[string]any
	if err := json.Unmarshal([]byte(payloadStr), &inner); err != nil {
		return p
	}
	if action, ok := inner["action"].(string); ok {
		p.Path = action
	}
	// correlationId may also live one layer deeper (depending on CLI version).
	if p.CorrelationID == "" {
		if cid, ok := inner["correlationId"].(string); ok {
			p.CorrelationID = cid
		}
	}
	if payloadField, ok := inner["payload"].(map[string]any); ok {
		p.Payload = payloadField
	}
	return p
}

// signalResult delivers the creature's response back to the requester user
// over the host's signaling channel. The CLI's WS listener picks up packets
// with this key and matches them by correlationId.
func signalResult(p packet, resp map[string]any) {
	if p.RequesterID == "" {
		return
	}
	if p.CorrelationID != "" {
		resp["correlationId"] = p.CorrelationID
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return
	}
	hostReq("signalUser", map[string]any{
		"key":    "creatures/signal/result",
		"userId": p.RequesterID,
		"packet": string(body),
		"system": true,
	})
}

func process(input string) string {
	p := unwrapSignal(input)
	setHostContext(p.CreatureID, p.Payload)
	hostReq("putJson", map[string]any{"key": "Json::CreatureNamespace::spaces", "path": "lastInput", "data": p.Payload, "merge": true})
	if p.Path != "" {
		hostReq("dbOp", map[string]any{"op": "put", "key": "creatureNamespace::spaces::lastPath", "val": p.Path})
	}

	// Dispatch the namespace action by `path`. The caller (typically
	// /creatures/signal with action="create" etc.) sets payload.path on the
	// signal envelope; that becomes p.Path here.
	resp := map[string]any{"ok": true, "namespace": "spaces", "action": p.Path}
	switch p.Path {
	case "create":
		creatorId := extractContextString(p.Payload, "creatorId", "userId", "creatureId")
		if creatorId == "" {
			creatorId = hostCreatureID
		}
		hostInput := map[string]any{
			"creatorId": creatorId,
			"isPublic":  p.Payload["isPublic"],
			"persHist":  p.Payload["persHist"],
			"tag":       p.Payload["origin"],
			"metadata":  p.Payload["metadata"],
		}
		raw := hostReq("createStore", hostInput)
		var hostResp map[string]any
		_ = json.Unmarshal([]byte(raw), &hostResp)
		resp["host"] = hostResp
		if sid, ok := hostResp["storeId"].(string); ok {
			resp["storeId"] = sid
		}
	case "delete":
		raw := hostReq("deleteStore", map[string]any{"storeId": extractContextString(p.Payload, "storeId")})
		var hostResp map[string]any
		_ = json.Unmarshal([]byte(raw), &hostResp)
		resp["host"] = hostResp
	case "get":
		raw := hostReq("getStore", map[string]any{"storeId": extractContextString(p.Payload, "storeId")})
		var hostResp map[string]any
		_ = json.Unmarshal([]byte(raw), &hostResp)
		resp["host"] = hostResp
	case "list":
		userId := extractContextString(p.Payload, "userId", "creatureId")
		if userId == "" {
			userId = hostCreatureID
		}
		raw := hostReq("listStores", map[string]any{"userId": userId})
		var hostResp map[string]any
		_ = json.Unmarshal([]byte(raw), &hostResp)
		resp["host"] = hostResp
	case "update":
		raw := hostReq("updateStore", map[string]any{
			"storeId":  extractContextString(p.Payload, "storeId"),
			"isPublic": p.Payload["isPublic"],
			"persHist": p.Payload["persHist"],
			"tag":      p.Payload["tag"],
			"metadata": p.Payload["metadata"],
		})
		var hostResp map[string]any
		_ = json.Unmarshal([]byte(raw), &hostResp)
		resp["host"] = hostResp
	}

	// Deliver the response back to the requester over the signaling channel.
	// The CLI's signalMiniapp listens for "creatures/signal/result" packets
	// and matches them by correlationId for its async req/res scheme.
	signalResult(p, resp)
	out, err := json.Marshal(resp)
	if err != nil {
		return ""
	}
	return string(out)
}

//export run
func run(arg uint64) int64 {
	input := stringAt(uint32(arg>>32), uint32(arg))
	process(input)
	return 0
}

func main() {}
