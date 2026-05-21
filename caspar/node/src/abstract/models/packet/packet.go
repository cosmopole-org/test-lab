package packet

type Packet struct {
	Origin string `json:"origin"`
	Data   string `json:"data"`
}

type LogPacket struct {
	Id      string `json:"id"`
	StoreId string `json:"storeId"`
	UserId  string `json:"userId"`
	Data    string `json:"data"`
	Time    int64  `json:"time"`
	Edited  bool   `json:"edited"`
}

type BuildPacket struct {
	Id         string `json:"id"`
	BuildId    string `json:"buildId"`
	CreatureId string `json:"creatureId"`
	VmId       string `json:"vmId,omitempty"`
	LogType    string `json:"logType,omitempty"`
	Time       int64  `json:"time"`
	Data       string `json:"data"`
}
