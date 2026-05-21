package inputs_machiner

type ReadVmLogsInput struct {
	VmId    string `json:"vmId" validate:"required"`
	LogType string `json:"logType"`
	Offset  int    `json:"offset"`
	Count   int    `json:"count"`
}

func (d ReadVmLogsInput) GetData() any {
	return "dummy"
}

func (d ReadVmLogsInput) GetStoreId() string {
	return ""
}

func (d ReadVmLogsInput) Origin() string {
	return "global"
}
