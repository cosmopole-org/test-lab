package model

import (
	"kasper/src/abstract/models/trx"
)

type File struct {
	Id      string `json:"id"`
	StoreId string `json:"storeId"`
	OwnerId string `json:"senderId"`
}

func (d File) Type() string {
	return "File"
}

func (d File) Push(trx trx.ITrx) {
	trx.PutObj(d.Type(), d.Id, map[string][]byte{
		"storeId": []byte(d.StoreId),
		"ownerId": []byte(d.OwnerId),
	})
}

func (d File) Pull(trx trx.ITrx) File {
	m := trx.GetObj(d.Type(), d.Id)
	if len(m) > 0 {
		d.StoreId = string(m["storeId"])
		d.OwnerId = string(m["ownerId"])
	}
	return d
}
