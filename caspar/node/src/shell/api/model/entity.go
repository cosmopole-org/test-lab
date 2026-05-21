package model

import "kasper/src/abstract/models/trx"

type Entity struct {
	ProgramId  string `json:"programId"`
	EntityId   string `json:"entityId"`
	EntityType string `json:"entityType"`
	ImageName  string `json:"imageName"`
}

func (m Entity) Type() string {
	return "Entity"
}

func (m Entity) Key() string {
	return m.ProgramId + "::" + m.EntityId
}

func (d Entity) Push(trx trx.ITrx) {
	trx.PutObj(d.Type(), d.Key(), map[string][]byte{
		"programId":  []byte(d.ProgramId),
		"entityId":   []byte(d.EntityId),
		"entityType": []byte(d.EntityType),
		"imageName":  []byte(d.ImageName),
	})
}

func (d Entity) Pull(trx trx.ITrx) Entity {
	m := trx.GetObj(d.Type(), d.Key())
	if len(m) > 0 {
		d.ProgramId = string(m["programId"])
		d.EntityId = string(m["entityId"])
		d.EntityType = string(m["entityType"])
		d.ImageName = string(m["imageName"])
	}
	return d
}
