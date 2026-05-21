package inputs_users

type LockTokenInput struct {
	Amount   int64                `json:"amount,omitempty"`
	Type     string               `json:"type" validate:"required"`
	Target   string               `json:"target" validate:"required"`
	UnlockAt int64                `json:"unlockAt,omitempty"`
	Steps    []LockTokenStepInput `json:"steps,omitempty"`
}

type LockTokenStepInput struct {
	Amount   int64 `json:"amount" validate:"required"`
	UnlockAt int64 `json:"unlockAt" validate:"required"`
}

func (d LockTokenInput) GetData() any {
	return "dummy"
}

func (d LockTokenInput) GetStoreId() string {
	return ""
}

func (d LockTokenInput) Origin() string {
	return "global"
}
