package updates_stores

import model "kasper/src/shell/api/model"

type Send struct {
	User     model.User  `json:"user"`
	Store    model.Store `json:"store,omitempty"`
	Action   string      `json:"action"`
	Data     string      `json:"data"`
	IsTemp   bool        `json:"isTemp,omitempty"`
	EntityId string      `json:"entityId,omitempty"`
}

type Update struct {
	Store model.Store `json:"store"`
}

type Delete struct {
	Store model.Store `json:"store"`
}

type AddMember struct {
	StoreId string     `json:"storeId"`
	User    model.User `json:"user"`
}

type UpdateMember struct {
	StoreId  string         `json:"storeId"`
	User     model.User     `json:"user"`
	Metadata map[string]any `json:"metadata"`
}

type Join struct {
	StoreId string     `json:"storeId"`
	User    model.User `json:"user"`
}
