package outputs_stores

import model "kasper/src/shell/api/model"

type AdminPoiint struct {
	Store model.Store `json:"store"`
	Admin bool        `json:"admin"`
}

type CreateOutput struct {
	Store AdminPoiint `json:"store"`
}
