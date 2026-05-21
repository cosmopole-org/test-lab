package secured

import (
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	model "kasper/src/core/module/actor/model/base"
)

type Guard struct {
	IsUser    bool `json:"isUser"`
	IsInStore bool `json:"isInStore"`
}

func (g *Guard) optionalIdentity(app core.ICore, packet []byte, signature string, userId string, storeId string) (bool, *model.Info) {
	if userId == "" && signature == "" {
		return true, model.NewInfo("", "")
	}
	if userId == "" || signature == "" {
		return false, &model.Info{}
	}
	identified, _, isGod := app.Tools().Security().AuthWithSignature(userId, packet, signature)
	if !identified {
		return false, &model.Info{}
	}
	if !g.IsInStore {
		return true, model.NewGodInfo(userId, "", isGod)
	}
	hasAccess := app.Tools().Security().HasAccessToStore(userId, storeId)
	if !hasAccess {
		return false, &model.Info{}
	}
	return true, model.NewGodInfo(userId, storeId, isGod)
}

func (g *Guard) CheckValidity(app core.ICore, packet []byte, signature string, userId string, storeId string, insider ...bool) (bool, *model.Info) {
	if !g.IsUser {
		return g.optionalIdentity(app, packet, signature, userId, storeId)
	}
	if len(insider) > 0 && insider[0] && (signature == "#appletsign") {
		typ := ""
		app.ModifyState(true, func(trx trx.ITrx) error {
			typ = string(trx.GetColumn("User", userId, "type"))
			return nil
		})
		if typ == "machine" {
			if !g.IsInStore {
				return true, model.NewGodInfo(userId, "", false)
			}
			hasAccess := app.Tools().Security().HasAccessToStore(userId, storeId)
			if !hasAccess {
				return false, &model.Info{}
			}
			return true, model.NewGodInfo(userId, storeId, false)
		}
	}
	identified, _, isGod := app.Tools().Security().AuthWithSignature(userId, packet, signature)
	if !identified {
		return false, &model.Info{}
	}
	if !g.IsInStore {
		return true, model.NewGodInfo(userId, "", isGod)
	}
	hasAccess := app.Tools().Security().HasAccessToStore(userId, storeId)
	if !hasAccess {
		return false, &model.Info{}
	}
	return true, model.NewGodInfo(userId, storeId, isGod)
}

func (g *Guard) CheckValidityForChain(app core.ICore, packet []byte, signature string, userId string, storeId string) (bool, *model.Info) {
	if !g.IsUser {
		return g.optionalIdentity(app, packet, signature, userId, storeId)
	}
	if signature == "#appletsign" {
		typ := ""
		app.ModifyState(true, func(trx trx.ITrx) error {
			typ = string(trx.GetColumn("User", userId, "type"))
			return nil
		})
		if typ == "machine" {
			if !g.IsInStore {
				return true, model.NewGodInfo(userId, "", false)
			}
			hasAccess := app.Tools().Security().HasAccessToStore(userId, storeId)
			if !hasAccess {
				return false, &model.Info{}
			}
			return true, model.NewGodInfo(userId, storeId, false)
		}
	}
	identified, _, isGod := app.Tools().Security().AuthWithSignature(userId, packet, signature)
	if !identified {
		return false, &model.Info{}
	}
	if !g.IsInStore {
		return true, model.NewGodInfo(userId, "", isGod)
	}
	hasAccess := app.Tools().Security().HasAccessToStore(userId, storeId)
	if !hasAccess {
		return false, &model.Info{}
	}
	return true, model.NewGodInfo(userId, storeId, isGod)
}

func (g *Guard) CheckIdentity(app core.ICore, packet []byte, signature string, userId string) bool {
	if !g.IsUser {
		if userId == "" && signature == "" {
			return true
		}
		if userId == "" || signature == "" {
			return false
		}
		identified, _, _ := app.Tools().Security().AuthWithSignature(userId, packet, signature)
		return identified
	}
	identified, _, _ := app.Tools().Security().AuthWithSignature(userId, packet, signature)
	return identified
}
