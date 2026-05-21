package model

import (
	"kasper/src/abstract/models/trx"
	"log"
	"sort"
)

type Chain struct {
	Id      string `json:"id"`
	StoreId string `json:"storeId"`
}

func (d Chain) Type() string {
	return "Chain"
}

func (d Chain) Push(trx trx.ITrx) {
	trx.PutObj(d.Type(), d.Id, map[string][]byte{
		"id":      []byte(d.Id),
		"storeId": []byte(d.StoreId),
	})
}

func (d Chain) Delete(trx trx.ITrx) {
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::|")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::id")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::storeId")
}

func (d Chain) Pull(trx trx.ITrx) Chain {
	m := trx.GetObj(d.Type(), d.Id)
	if len(m) > 0 {
		d.Id = string(m["id"])
		d.StoreId = string(m["storeId"])
	}
	return d
}

func (d Chain) All(trx trx.ITrx, offset int64, count int64, query map[string]string) ([]Chain, error) {
	objs, err := trx.GetObjList("Chain", []string{"*"}, query, offset, count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []Chain{}
	for id, m := range objs {
		if len(m) > 0 {
			d := Chain{}
			d.Id = id
			d.StoreId = string(m["storeId"])
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}

type ChainShard struct {
	Id          string `json:"id"`
	WorkChainId string `json:"workChainId"`
}

func (d ChainShard) Type() string {
	return "ChainShard"
}

func (d ChainShard) Push(trx trx.ITrx) {
	trx.PutObj(d.Type(), d.Id, map[string][]byte{
		"id":          []byte(d.Id),
		"workChainId": []byte(d.WorkChainId),
	})
}

func (d ChainShard) Delete(trx trx.ITrx) {
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::|")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::id")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::workChainId")
}

func (d ChainShard) Pull(trx trx.ITrx) ChainShard {
	m := trx.GetObj(d.Type(), d.Id)
	if len(m) > 0 {
		d.Id = string(m["id"])
		d.WorkChainId = string(m["workChainId"])
	}
	return d
}

func (d ChainShard) All(trx trx.ITrx, offset int64, count int64, query map[string]string) ([]ChainShard, error) {
	objs, err := trx.GetObjList("ChainShard", []string{"*"}, query, offset, count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []ChainShard{}
	for id, m := range objs {
		if len(m) > 0 {
			d := ChainShard{}
			d.Id = id
			d.WorkChainId = string(m["workChainId"])
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].WorkChainId == entities[j].WorkChainId {
			return entities[i].Id < entities[j].Id
		}
		return entities[i].WorkChainId < entities[j].WorkChainId
	})
	return entities, nil
}
