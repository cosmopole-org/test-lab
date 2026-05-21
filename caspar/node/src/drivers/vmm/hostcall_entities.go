package vmm

import (
	"encoding/json"
	"fmt"
	iaction "kasper/src/abstract/models/action"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/state"
	"kasper/src/core/module/actor/model/base"
	model "kasper/src/shell/api/model"
	"os"
	"path/filepath"
	"strings"
)

func numberFromInput(input map[string]any, key string, def int64) int64 {
	raw, ok := input[key]
	if !ok {
		return def
	}
	switch v := raw.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return def
	}
}

func (wm *Vmm) handleCreatureCrud(op string, input map[string]any, reqId int64) (string, int64) {
	switch op {
	case "create":
		id, _ := checkField(input, "id", "")
		if id == "" {
			wm.app.ModifyState(true, func(t trx.ITrx) error {
				id = wm.app.Tools().Storage().GenId(t, "vm.creature")
				return nil
			})
		}
		typ, _ := checkField(input, "type", "agent")
		username, _ := checkField(input, "username", "")
		publicKey, _ := checkField(input, "publicKey", "")
		chainId, _ := checkField(input, "chainId", "main")
		subchainId, _ := checkField(input, "subchainId", "main")
		ownerId, _ := checkField(input, "ownerId", "")
		balance := numberFromInput(input, "balance", 0)
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			c := model.Creature{
				Id:         id,
				TypeName:   typ,
				Username:   username,
				PublicKey:  publicKey,
				ChainId:    chainId,
				SubchainId: subchainId,
				OwnerId:    ownerId,
				Balance:    balance,
			}
			c.Push(t)
			if ownerId != "" {
				t.PutLink("ownerof::"+ownerId+"::"+id, "true")
			}
			return nil
		})
		return fmt.Sprintf(`{"ok":true,"id":"%s"}`, id), reqId
	case "update":
		id, err := checkField(input, "id", "")
		if err != nil || id == "" {
			return `{"ok":false,"error":"id is required"}`, reqId
		}
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			c := model.Creature{Id: id}.Pull(t)
			if c.Id == "" {
				return nil
			}
			if v, ok := input["type"].(string); ok {
				c.TypeName = v
			}
			if v, ok := input["username"].(string); ok {
				c.Username = v
			}
			if v, ok := input["publicKey"].(string); ok {
				c.PublicKey = v
			}
			if v, ok := input["chainId"].(string); ok {
				c.ChainId = v
			}
			if v, ok := input["subchainId"].(string); ok {
				c.SubchainId = v
			}
			if v, ok := input["ownerId"].(string); ok {
				c.OwnerId = v
			}
			if v, ok := input["balance"].(float64); ok {
				c.Balance = int64(v)
			}
			c.Push(t)
			return nil
		})
		return fmt.Sprintf(`{"ok":true,"id":"%s"}`, id), reqId
	case "delete":
		id, err := checkField(input, "id", "")
		if err != nil || id == "" {
			return `{"ok":false,"error":"id is required"}`, reqId
		}
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			c := model.Creature{Id: id}.Pull(t)
			if c.Username != "" {
				t.DelIndex("Creature", "username", "id", c.Username)
			}
			t.DelKey("obj::Creature::" + id + "::|")
			t.DelKey("obj::Creature::" + id + "::type")
			t.DelKey("obj::Creature::" + id + "::username")
			t.DelKey("obj::Creature::" + id + "::publicKey")
			t.DelKey("obj::Creature::" + id + "::chainId")
			t.DelKey("obj::Creature::" + id + "::subchainId")
			t.DelKey("obj::Creature::" + id + "::ownerId")
			t.DelKey("obj::Creature::" + id + "::balance")
			return nil
		})
		return fmt.Sprintf(`{"ok":true,"id":"%s"}`, id), reqId
	case "get":
		id, err := checkField(input, "id", "")
		if err != nil || id == "" {
			return `{"ok":false,"error":"id is required"}`, reqId
		}
		out := map[string]any{"ok": true}
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			out["creature"] = model.Creature{Id: id}.Pull(t)
			return nil
		})
		b, _ := json.Marshal(out)
		return string(b), reqId
	case "list":
		offset := numberFromInput(input, "offset", 0)
		count := numberFromInput(input, "count", 100)
		if count <= 0 {
			count = 100
		}
		out := map[string]any{"ok": true, "creatures": []model.Creature{}}
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			creatures, _ := model.Creature{}.All(t, offset, count)
			out["creatures"] = creatures
			return nil
		})
		b, _ := json.Marshal(out)
		return string(b), reqId
	}
	return `{"ok":false,"error":"unsupported creature op"}`, reqId
}

func (wm *Vmm) handleResourceStoreCrud(op string, input map[string]any, reqId int64) (string, int64) {
	switch op {
	case "create", "update":
		storeId, _ := checkField(input, "storeId", "")
		machineId, _ := checkField(input, "machineId", "")
		if storeId == "" {
			wm.app.ModifyState(true, func(t trx.ITrx) error {
				storeId = wm.app.Tools().Storage().GenId(t, "vm.store")
				return nil
			})
		}
		name, _ := checkField(input, "name", storeId)
		metadata := map[string]any{}
		if raw, ok := input["metadata"].(map[string]any); ok {
			metadata = raw
		}
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			key := "Json::VmResourceStore::" + storeId
			_ = t.PutJson(key, "metadata", metadata, true)
			_ = t.PutJson(key, "core", map[string]any{
				"id":        storeId,
				"name":      name,
				"machineId": machineId,
			}, true)
			if machineId != "" {
				t.PutLink("vmOwnedStore::"+machineId+"::"+storeId, "true")
			}
			return nil
		})
		return fmt.Sprintf(`{"ok":true,"storeId":"%s"}`, storeId), reqId
	case "delete":
		storeId, err := checkField(input, "storeId", "")
		if err != nil || storeId == "" {
			return `{"ok":false,"error":"storeId is required"}`, reqId
		}
		machineId, _ := checkField(input, "machineId", "")
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			t.DelKey("Json::VmResourceStore::" + storeId + "::metadata")
			t.DelKey("Json::VmResourceStore::" + storeId + "::core")
			if machineId != "" {
				t.DelKey("link::vmOwnedStore::" + machineId + "::" + storeId)
			}
			return nil
		})
		return fmt.Sprintf(`{"ok":true,"storeId":"%s"}`, storeId), reqId
	case "get":
		storeId, err := checkField(input, "storeId", "")
		if err != nil || storeId == "" {
			return `{"ok":false,"error":"storeId is required"}`, reqId
		}
		out := map[string]any{"ok": true, "store": map[string]any{}}
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			core, _ := t.GetJson("Json::VmResourceStore::"+storeId, "core")
			metadata, _ := t.GetJson("Json::VmResourceStore::"+storeId, "metadata")
			out["store"] = map[string]any{"core": core, "metadata": metadata}
			return nil
		})
		b, _ := json.Marshal(out)
		return string(b), reqId
	case "list":
		machineId, _ := checkField(input, "machineId", "")
		out := map[string]any{"ok": true, "stores": []string{}}
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			prefix := "link::vmOwnedStore::" + machineId + "::"
			if machineId == "" {
				prefix = "Json::VmResourceStore::"
			}
			out["stores"] = t.GetByPrefix(prefix)
			return nil
		})
		b, _ := json.Marshal(out)
		return string(b), reqId
	}
	return `{"ok":false,"error":"unsupported store op"}`, reqId
}

func (wm *Vmm) handleResourceEntityCreate(input map[string]any, reqId int64) (string, int64) {
	storeId, err := checkField(input, "storeId", "")
	if err != nil || storeId == "" {
		return `{"ok":false,"error":"storeId is required"}`, reqId
	}
	entityType, _ := checkField(input, "entityType", "default")
	entityId, _ := checkField(input, "entityId", "")
	if entityId == "" {
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			entityId = wm.app.Tools().Storage().GenId(t, "vm.entity")
			return nil
		})
	}
	payload := map[string]any{}
	if raw, ok := input["payload"].(map[string]any); ok {
		payload = raw
	}
	data, _ := checkField(input, "data", "")
	basePath := filepath.Join(wm.storageRoot, "vm_stores", storeId, entityType)
	_ = os.MkdirAll(basePath, os.ModePerm)
	path := filepath.Join(basePath, entityId+".json")
	_ = os.WriteFile(path, []byte(data), 0644)

	wm.app.ModifyState(false, func(t trx.ITrx) error {
		key := fmt.Sprintf("Json::VmResourceEntity::%s::%s::%s", storeId, entityType, entityId)
		_ = t.PutJson(key, "payload", payload, true)
		_ = t.PutJson(key, "meta", map[string]any{
			"id":         entityId,
			"storeId":    storeId,
			"entityType": entityType,
			"path":       path,
		}, true)
		return nil
	})
	return fmt.Sprintf(`{"ok":true,"entityId":"%s","path":"%s"}`, entityId, path), reqId
}

func (wm *Vmm) handleResourceEntityDelete(input map[string]any, reqId int64) (string, int64) {
	storeId, err := checkField(input, "storeId", "")
	if err != nil || storeId == "" {
		return `{"ok":false,"error":"storeId is required"}`, reqId
	}
	entityType, _ := checkField(input, "entityType", "default")
	entityId, err := checkField(input, "entityId", "")
	if err != nil || entityId == "" {
		return `{"ok":false,"error":"entityId is required"}`, reqId
	}
	path := filepath.Join(wm.storageRoot, "vm_stores", storeId, entityType, entityId+".json")
	_ = os.Remove(path)
	wm.app.ModifyState(false, func(t trx.ITrx) error {
		key := fmt.Sprintf("Json::VmResourceEntity::%s::%s::%s", storeId, entityType, entityId)
		t.DelKey(key + "::payload")
		t.DelKey(key + "::meta")
		return nil
	})
	return `{"ok":true}`, reqId
}

func (wm *Vmm) handleVmChainRequest(op string, input map[string]any, reqId int64) (string, int64) {
	storeId, _ := checkField(input, "storeId", "")
	receivers := map[string]map[string]bool{"*": {}}
	if op == "createWorkchain" {
		chainId := wm.app.Tools().Network().Chain().CreateWorkChain(storeId)
		payload, _ := json.Marshal(map[string]any{"op": op, "chainId": chainId, "storeId": storeId})
		wm.app.Globe().SendTypedMessageOnChain("main", "chains/vm/request", "vm.chain", payload, "", wm.app.OwnerId(), receivers, "", storeId, nil, nil)
		return fmt.Sprintf(`{"ok":true,"chainId":"%s"}`, chainId), reqId
	}
	if op == "createSubchain" {
		workChainId, _ := checkField(input, "workChainId", "")
		subchainId, _ := checkField(input, "subchainId", "")
		peers := []string{}
		if peersRaw, ok := input["peers"].([]any); ok {
			for _, p := range peersRaw {
				if s, ok := p.(string); ok {
					peers = append(peers, s)
				}
			}
		}
		subchainId = wm.app.Tools().Network().Chain().CreateShardChain(workChainId, subchainId, peers)
		payload, _ := json.Marshal(map[string]any{"op": op, "workChainId": workChainId, "subchainId": subchainId, "peers": peers})
		wm.app.Globe().SendTypedMessageOnChain("main", "chains/vm/request", "vm.chain", payload, "", wm.app.OwnerId(), receivers, "", storeId, nil, nil)
		return fmt.Sprintf(`{"ok":true,"workChainId":"%s","subchainId":"%s"}`, workChainId, subchainId), reqId
	}
	if strings.HasPrefix(op, "delete") {
		payload, _ := json.Marshal(map[string]any{"op": op, "input": input})
		wm.app.Globe().SendTypedMessageOnChain("main", "chains/vm/request", "vm.chain", payload, "", wm.app.OwnerId(), receivers, "", storeId, nil, nil)
		return `{"ok":true,"notified":true}`, reqId
	}
	return `{"ok":false,"error":"unsupported chain op"}`, reqId
}

func (wm *Vmm) handleExecShellAction(input map[string]any, reqId int64) (string, int64) {
	path, err := checkField(input, "path", "")
	if err != nil || path == "" {
		return `{"ok":false,"error":"path is required"}`, reqId
	}
	userId, _ := checkField(input, "userId", wm.app.OwnerId())
	storeId, _ := checkField(input, "storeId", "")
	signature, _ := checkField(input, "signature", "")
	packetID, _ := checkField(input, "packetId", "")

	action := wm.app.Actor().FetchAction(path)
	if action == nil {
		return `{"ok":false,"error":"action not found"}`, reqId
	}
	secureAction, ok := action.(iaction.ISecureAction)
	if !ok {
		return `{"ok":false,"error":"action is not secure"}`, reqId
	}

	payloadRaw, _ := input["payload"]
	payloadBytes, _ := json.Marshal(payloadRaw)
	parsedInput, parseErr := secureAction.ParseInput("tcp", payloadRaw)
	if parseErr != nil {
		return fmt.Sprintf(`{"ok":false,"error":%q}`, parseErr.Error()), reqId
	}

	statusCode := 0
	var result any = map[string]any{}
	var actErr error
	wm.app.ModifyStateSecurly(false, base.NewInfo(userId, storeId), func(_ state.IState) error {
		statusCode, result, actErr = secureAction.SecurelyAct(userId, packetID, payloadBytes, signature, parsedInput, wm.app.IpAddr(), true)
		return nil
	})
	if actErr != nil {
		return fmt.Sprintf(`{"ok":false,"statusCode":%d,"error":%q}`, statusCode, actErr.Error()), reqId
	}
	res, _ := json.Marshal(map[string]any{"ok": true, "statusCode": statusCode, "result": result})
	return string(res), reqId
}

func (wm *Vmm) handleMicroHostAction(op string, input map[string]any, reqId int64) (string, int64) {
	switch op {
	case "genId":
		source, _ := checkField(input, "source", "vm.micro")
		id := ""
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			id = wm.app.Tools().Storage().GenId(t, source)
			return nil
		})
		return fmt.Sprintf(`{"ok":true,"id":"%s"}`, id), reqId
	case "getLink":
		key, err := checkField(input, "key", "")
		if err != nil || key == "" {
			return `{"ok":false,"error":"key is required"}`, reqId
		}
		val := ""
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			val = t.GetLink(key)
			return nil
		})
		return fmt.Sprintf(`{"ok":true,"value":%q}`, val), reqId
	case "delKey":
		key, err := checkField(input, "key", "")
		if err != nil || key == "" {
			return `{"ok":false,"error":"key is required"}`, reqId
		}
		if strings.HasPrefix(key, "link::") {
			return `{"ok":false,"error":"link modifications are not allowed via delKey"}`, reqId
		}
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			t.DelKey(key)
			return nil
		})
		return `{"ok":true}`, reqId
	case "createAccess":
		userId, err := checkField(input, "userId", "")
		if err != nil || userId == "" {
			return `{"ok":false,"error":"userId is required"}`, reqId
		}
		storeId, err := checkField(input, "storeId", "")
		if err != nil || storeId == "" {
			return `{"ok":false,"error":"storeId is required"}`, reqId
		}
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			t.PutLink("onaccess::"+storeId+"::"+userId, "true")
			t.PutLink("hasaccess::"+userId+"::"+storeId, "true")
			return nil
		})
		return `{"ok":true}`, reqId
	case "deleteAccess":
		userId, err := checkField(input, "userId", "")
		if err != nil || userId == "" {
			return `{"ok":false,"error":"userId is required"}`, reqId
		}
		storeId, err := checkField(input, "storeId", "")
		if err != nil || storeId == "" {
			return `{"ok":false,"error":"storeId is required"}`, reqId
		}
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			t.DelKey("link::onaccess::" + storeId + "::" + userId)
			t.DelKey("link::hasaccess::" + userId + "::" + storeId)
			return nil
		})
		return `{"ok":true}`, reqId
	case "getJson":
		key, err := checkField(input, "key", "")
		if err != nil || key == "" {
			return `{"ok":false,"error":"key is required"}`, reqId
		}
		path, _ := checkField(input, "path", "")
		jsonRes := map[string]any{}
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			jsonRes, _ = t.GetJson(key, path)
			return nil
		})
		b, _ := json.Marshal(map[string]any{"ok": true, "data": jsonRes})
		return string(b), reqId
	case "putJson":
		key, err := checkField(input, "key", "")
		if err != nil || key == "" {
			return `{"ok":false,"error":"key is required"}`, reqId
		}
		path, _ := checkField(input, "path", "")
		merge, _ := checkField(input, "merge", true)
		obj, _ := input["data"]
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			_ = t.PutJson(key, path, obj, merge)
			return nil
		})
		return `{"ok":true}`, reqId
	case "getByPrefix":
		prefix, err := checkField(input, "prefix", "")
		if err != nil {
			return `{"ok":false,"error":"prefix is required"}`, reqId
		}
		results := []string{}
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			results = t.GetByPrefix(prefix)
			return nil
		})
		b, _ := json.Marshal(map[string]any{"ok": true, "data": results})
		return string(b), reqId
	case "hasAccessToStore":
		machineId, err := checkField(input, "machineId", "")
		if err != nil || machineId == "" {
			return `{"ok":false,"error":"machineId is required"}`, reqId
		}
		storeId, err := checkField(input, "storeId", "")
		if err != nil || storeId == "" {
			return `{"ok":false,"error":"storeId is required"}`, reqId
		}
		allowed := wm.app.Tools().Security().HasAccessToStore(machineId, storeId)
		b, _ := json.Marshal(map[string]any{"ok": true, "allowed": allowed})
		return string(b), reqId
	case "signalUser":
		key, _ := checkField(input, "key", "")
		userId, _ := checkField(input, "userId", "")
		packet, _ := checkField(input, "packet", "{}")
		isSystem, _ := checkField(input, "system", true)
		wm.app.Tools().Signaler().SignalUser(key, userId, []byte(packet), isSystem)
		return `{"ok":true}`, reqId
	case "signalGroup":
		key, _ := checkField(input, "key", "")
		groupId, _ := checkField(input, "groupId", "")
		packet, _ := checkField(input, "packet", "{}")
		isSystem, _ := checkField(input, "system", true)
		except := []string{}
		if exceptRaw, ok := input["except"].([]any); ok {
			for _, ex := range exceptRaw {
				if exS, ok := ex.(string); ok {
					except = append(except, exS)
				}
			}
		}
		wm.app.Tools().Signaler().SignalGroup(key, groupId, []byte(packet), isSystem, except)
		return `{"ok":true}`, reqId
	case "joinGroup":
		groupId, _ := checkField(input, "groupId", "")
		userId, _ := checkField(input, "userId", "")
		wm.app.Tools().Signaler().JoinGroup(groupId, userId)
		return `{"ok":true}`, reqId
	default:
		return `{"ok":false,"error":"unsupported micro op"}`, reqId
	}
}

// handleStoreCrud implements the collaboration-Store CRUD ops exposed to VMs
// via host calls (op keys: createStore, updateStore, deleteStore, getStore,
// listStores). These manage rows in the Store model (collaboration surfaces
// users can join, send signals to, etc.) — distinct from the VM-resource
// stores handled by handleResourceStoreCrud.
func (wm *Vmm) handleStoreCrud(op string, input map[string]any, reqId int64) (string, int64) {
	switch op {
	case "create":
		storeId, _ := checkField(input, "storeId", "")
		creatorId, _ := checkField(input, "creatorId", "")
		if creatorId == "" {
			creatorId, _ = checkField(input, "userId", "")
		}
		tag, _ := checkField(input, "tag", "")
		parentId, _ := checkField(input, "parentId", "")
		isPublic := boolFromInput(input, "isPublic", false)
		persHist := boolFromInput(input, "persHist", false)
		metadata := map[string]any{}
		if raw, ok := input["metadata"].(map[string]any); ok {
			metadata = raw
		}
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			if storeId == "" {
				storeId = wm.app.Tools().Storage().GenId(t, "store")
			}
			store := model.Store{
				Id:          storeId,
				Tag:         tag,
				ParentId:    parentId,
				IsPublic:    isPublic,
				PersHist:    persHist,
				MemberCount: 1,
			}
			store.Push(t)
			_ = t.PutJson("StoreMeta::"+storeId, "metadata", metadata, true)
			if creatorId != "" {
				t.PutLink("hasaccess::"+creatorId+"::"+storeId, "true")
				t.PutLink("creatorof::"+creatorId+"::"+storeId, "true")
				t.PutLink("onaccess::"+storeId+"::"+creatorId, "true")
			}
			return nil
		})
		out, _ := json.Marshal(map[string]any{"ok": true, "storeId": storeId})
		return string(out), reqId
	case "update":
		storeId, err := checkField(input, "storeId", "")
		if err != nil || storeId == "" {
			return `{"ok":false,"error":"storeId is required"}`, reqId
		}
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			store := model.Store{Id: storeId}.Pull(t)
			if store.Id == "" {
				return nil
			}
			if v, ok := input["isPublic"].(bool); ok {
				store.IsPublic = v
			}
			if v, ok := input["persHist"].(bool); ok {
				store.PersHist = v
			}
			if v, ok := input["tag"].(string); ok {
				store.Tag = v
			}
			store.Push(t)
			if md, ok := input["metadata"].(map[string]any); ok {
				_ = t.PutJson("StoreMeta::"+storeId, "metadata", md, true)
			}
			return nil
		})
		return fmt.Sprintf(`{"ok":true,"storeId":"%s"}`, storeId), reqId
	case "delete":
		storeId, err := checkField(input, "storeId", "")
		if err != nil || storeId == "" {
			return `{"ok":false,"error":"storeId is required"}`, reqId
		}
		wm.app.ModifyState(false, func(t trx.ITrx) error {
			store := model.Store{Id: storeId}.Pull(t)
			if store.Id != "" {
				store.Delete(t)
			}
			t.DelKey("Json::StoreMeta::" + storeId + "::metadata")
			return nil
		})
		return fmt.Sprintf(`{"ok":true,"storeId":"%s"}`, storeId), reqId
	case "get":
		storeId, err := checkField(input, "storeId", "")
		if err != nil || storeId == "" {
			return `{"ok":false,"error":"storeId is required"}`, reqId
		}
		var store model.Store
		var meta map[string]any
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			store = model.Store{Id: storeId}.Pull(t)
			meta, _ = t.GetJson("StoreMeta::"+storeId, "metadata")
			return nil
		})
		out, _ := json.Marshal(map[string]any{"ok": true, "store": store, "metadata": meta})
		return string(out), reqId
	case "list":
		userId, _ := checkField(input, "userId", "")
		offset := int(numberFromInput(input, "offset", 0))
		count := int(numberFromInput(input, "count", 50))
		_ = offset
		_ = count
		out := map[string]any{"ok": true, "stores": []any{}}
		wm.app.ModifyState(true, func(t trx.ITrx) error {
			prefix := "hasaccess::" + userId + "::"
			if userId == "" {
				prefix = "obj::Store::"
			}
			stores, _ := model.Store{}.List(t, prefix, false, map[string]string{}, map[string][]string{})
			out["stores"] = stores
			return nil
		})
		b, _ := json.Marshal(out)
		return string(b), reqId
	}
	return `{"ok":false,"error":"unsupported store op"}`, reqId
}

func boolFromInput(input map[string]any, key string, def bool) bool {
	if v, ok := input[key].(bool); ok {
		return v
	}
	if v, ok := input[key].(string); ok {
		return v == "true" || v == "1"
	}
	return def
}
