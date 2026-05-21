package base

import "strings"

type Info struct {
	isGod   bool
	userId  string
	storeId string
}

func NewInfo(userId string, storeId string) *Info {
	return &Info{isGod: false, userId: userId, storeId: storeId}
}

func NewGodInfo(userId string, storeId string, isGod bool) *Info {
	return &Info{isGod: isGod, userId: userId, storeId: storeId}
}

func (info *Info) IsGod() bool {
	return info.isGod
}

func (info *Info) UserId() string {
	return info.userId
}

func (info *Info) StoreId() string {
	return info.storeId
}

func (info *Info) Identity() (string, string) {
	identity := strings.Split(info.userId, "@")
	if len(identity) == 2 {
		return identity[0], identity[1]
	}
	return "", ""
}