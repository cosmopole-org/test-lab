package model

import (
	"encoding/binary"
	"kasper/src/abstract/models/trx"
	"log"
	"sort"
)

type Machine struct {
	Id            string `json:"id"`
	ChainId       string `json:"chainId"`
	ShardChainId  string `json:"shardChainId"`
	OwnerId       string `json:"ownerId"`
	Username      string `json:"username"`
	MachinesCount int    `json:"machinesCount"`
	Title         string `json:"title"`
	Avatar        string `json:"avatar"`
	Desc          string `json:"desc"`
}

func (m Machine) Type() string {
	return "Machine"
}

func (d Machine) Push(trx trx.ITrx) {
	mcBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(mcBytes, uint32(d.MachinesCount))
	trx.PutObj(d.Type(), d.Id, map[string][]byte{
		"id":            []byte(d.Id),
		"ownerId":       []byte(d.OwnerId),
		"username":      []byte(d.Username),
		"chainId":       []byte(d.ChainId),
		"shardChainId":  []byte(d.ShardChainId),
		"machinesCount": mcBytes,
	})
}

func (d Machine) Pull(trx trx.ITrx, flags ...bool) Machine {
	m := trx.GetObj(d.Type(), d.Id)
	if len(m) > 0 {
		d.Id = string(m["id"])
		d.Username = string(m["username"])
		d.OwnerId = string(m["ownerId"])
		d.ChainId = string(m["chainId"])
		d.ShardChainId = string(m["shardChainId"])
		d.MachinesCount = int(binary.LittleEndian.Uint32(m["machinesCount"]))
		if len(flags) > 0 {
			if flags[0] {
				if metadata, err := trx.GetJson("MachineMeta::"+d.Id, "metadata.public.profile"); err == nil {
					d.Title = metadata["title"].(string)
					d.Avatar = metadata["avatar"].(string)
					d.Desc = metadata["desc"].(string)
				}
			}
		}
	}
	return d
}

func (d Machine) Delete(trx trx.ITrx) {
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::|")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::id")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::username")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::chainId")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::shardChainId")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::machinesCount")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::ownerId")
	trx.DelJson("MachineMeta::"+d.Id, "metadata")
}

func (d Machine) List(trx trx.ITrx, prefix string) ([]Machine, error) {
	list, err := trx.GetLinksList(prefix, -1, -1)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for i := 0; i < len(list); i++ {
		list[i] = list[i][len(prefix):]
	}
	objs, err := trx.GetObjList("Machine", list, map[string]string{})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []Machine{}
	for id, m := range objs {
		if len(m) > 0 {
			d := Machine{}
			d.Id = id
			d.OwnerId = string(m["ownerId"])
			d.Username = string(m["username"])
			d.ChainId = string(m["chainId"])
			d.ShardChainId = string(m["shardChainId"])
			d.MachinesCount = int(binary.LittleEndian.Uint32(m["machinesCount"]))
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}

func (d Machine) All(trx trx.ITrx, offset int64, count int64) ([]Machine, error) {
	objs, err := trx.GetObjList("Machine", []string{"*"}, map[string]string{}, offset, count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []Machine{}
	for id, m := range objs {
		if len(m) > 0 {
			d := Machine{}
			d.Id = id
			d.OwnerId = string(m["ownerId"])
			d.Username = string(m["username"])
			d.ChainId = string(m["chainId"])
			d.ShardChainId = string(m["shardChainId"])
			d.MachinesCount = int(binary.LittleEndian.Uint32(m["machinesCount"]))
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}

type Program struct {
	MachineId string `json:"id"`
	AppId     string `json:"appId"`
	Runtime   string `json:"runtime"`
	Path      string `json:"path"`
	Comment   string `json:"comment"`
}

func (m Program) Type() string {
	return "Program"
}

func (d Program) Push(trx trx.ITrx) {
	trx.PutObj(d.Type(), d.MachineId, map[string][]byte{
		"machineId": []byte(d.MachineId),
		"appId":     []byte(d.AppId),
		"runtime":   []byte(d.Runtime),
		"path":      []byte(d.Path),
		"comment":   []byte(d.Comment),
	})
}

func (d Program) Pull(trx trx.ITrx) Program {
	m := trx.GetObj(d.Type(), d.MachineId)
	if len(m) > 0 {
		d.MachineId = string(m["machineId"])
		d.AppId = string(m["appId"])
		d.Runtime = string(m["runtime"])
		d.Path = string(m["path"])
		d.Comment = string(m["comment"])
	}
	return d
}

func (d Program) All(trx trx.ITrx, offset int64, count int64) ([]Program, error) {
	if count == -1 {
		objs, err := trx.GetObjList("Program", []string{"*"}, map[string]string{})
		if err != nil {
			log.Println(err)
			return nil, err
		}
		entities := []Program{}
		for id, m := range objs {
			if len(m) > 0 {
				d := Program{}
				d.MachineId = id
				d.AppId = string(m["appId"])
				d.Runtime = string(m["runtime"])
				d.Path = string(m["path"])
				entities = append(entities, d)
			}
		}
		sort.Slice(entities, func(i, j int) bool {
			return entities[i].MachineId < entities[j].MachineId
		})
		return entities, nil
	} else {
		objs, err := trx.GetObjList("Program", []string{"*"}, map[string]string{}, offset, count)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		entities := []Program{}
		for id, m := range objs {
			if len(m) > 0 {
				d := Program{}
				d.MachineId = id
				d.AppId = string(m["appId"])
				d.Runtime = string(m["runtime"])
				d.Path = string(m["path"])
				d.Comment = string(m["comment"])
				entities = append(entities, d)
			}
		}
		sort.Slice(entities, func(i, j int) bool {
			return entities[i].MachineId < entities[j].MachineId
		})
		return entities, nil
	}
}

func (d Program) List(trx trx.ITrx, prefix string) ([]Program, error) {
	list, err := trx.GetLinksList(prefix, -1, -1)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for i := 0; i < len(list); i++ {
		list[i] = list[i][len(prefix):]
	}
	objs, err := trx.GetObjList("Program", list, map[string]string{})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []Program{}
	for id, m := range objs {
		if len(m) > 0 {
			d := Program{}
			d.MachineId = id
			d.AppId = string(m["appId"])
			d.Runtime = string(m["runtime"])
			d.Path = string(m["path"])
			d.Comment = string(m["comment"])
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].MachineId < entities[j].MachineId
	})
	return entities, nil
}
