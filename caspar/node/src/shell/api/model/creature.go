package model

import (
	"encoding/binary"
	"kasper/src/abstract/models/trx"
	"log"
	"sort"
)

type Creature struct {
	Id            string `json:"id"`
	TypeName      string `json:"type"`
	Username      string `json:"username,omitempty"`
	PublicKey     string `json:"publicKey,omitempty"`
	ChainId       string `json:"chainId"`
	SubchainId    string `json:"subchainId"`
	OwnerId       string `json:"ownerId,omitempty"`
	MachinesCount int    `json:"machinesCount,omitempty"`
	Title         string `json:"title,omitempty"`
	Avatar        string `json:"avatar,omitempty"`
	Desc          string `json:"desc,omitempty"`
	Balance       int64  `json:"balance"`
}

func (d Creature) Type() string { return "Creature" }

func (d Creature) Push(tx trx.ITrx) {
	bal := make([]byte, 8)
	binary.LittleEndian.PutUint64(bal, uint64(d.Balance))
	tx.PutObj(d.Type(), d.Id, map[string][]byte{
		"type":       []byte(d.TypeName),
		"username":   []byte(d.Username),
		"publicKey":  []byte(d.PublicKey),
		"chainId":    []byte(d.ChainId),
		"subchainId": []byte(d.SubchainId),
		"ownerId":    []byte(d.OwnerId),
		"balance":    bal,
	})
	if d.Username != "" {
		tx.PutIndex("Creature", "username", "id", d.Username, []byte(d.Id))
	}
}

func (d Creature) Pull(tx trx.ITrx) Creature {
	m := tx.GetObj(d.Type(), d.Id)
	if len(m) > 0 {
		d.TypeName = string(m["type"])
		d.Username = string(m["username"])
		d.PublicKey = string(m["publicKey"])
		d.ChainId = string(m["chainId"])
		d.SubchainId = string(m["subchainId"])
		d.OwnerId = string(m["ownerId"])
		if b, ok := m["balance"]; ok && len(b) == 8 {
			d.Balance = int64(binary.LittleEndian.Uint64(b))
		}
	}
	return d
}

func (d Creature) All(tx trx.ITrx, offset int64, count int64) ([]Creature, error) {
	objs, err := tx.GetObjList("Creature", []string{"*"}, map[string]string{}, offset, count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []Creature{}
	for id, m := range objs {
		if len(m) == 0 {
			continue
		}
		c := Creature{Id: id}
		c.TypeName = string(m["type"])
		c.Username = string(m["username"])
		c.PublicKey = string(m["publicKey"])
		c.ChainId = string(m["chainId"])
		c.SubchainId = string(m["subchainId"])
		c.OwnerId = string(m["ownerId"])
		if b, ok := m["balance"]; ok && len(b) == 8 {
			c.Balance = int64(binary.LittleEndian.Uint64(b))
		}
		entities = append(entities, c)
	}
	sort.Slice(entities, func(i, j int) bool { return entities[i].Id < entities[j].Id })
	return entities, nil
}
