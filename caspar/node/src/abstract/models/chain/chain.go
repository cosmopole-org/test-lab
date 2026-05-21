package chain

import "kasper/src/abstract/models/update"

type ChainMessage struct {
	Key         string
	MessageType string
	Author      string
	Submitter   string
	Payload     []byte
	Signatures  []string
	RequestId   string
	Recievers   map[string]map[string]bool
	ReplyTo     string
	StoreId     string
	Pay         *ChainPayPacket
}

type ChainPayPacket struct {
	Type             string
	SessionId        string
	MachineIds       []string
	UserId           string
	LockId           string
	LockSignature    string
	Amount           int64
	RequestedSeconds int64
	AcceptedSeconds  int64
	CostPerSecond    int64
	Error            string
	StoreId          string
	VmPayload        string
}

type ChainBaseRequest struct {
	Key        string
	Author     string
	Submitter  string
	Payload    []byte
	Signatures []string
	RequestId  string
	Tag        string
}

type ChainResponse struct {
	Executor  string
	Payload   []byte
	Signature string
	RequestId string
	Effects   Effects
	ResCode   int
	Err       string
	Tag       string
	ToUserId  string
}

type ChainAppletRequest struct {
	MachineId  string
	Key        string
	Author     string
	Submitter  string
	Payload    []byte
	Signatures []string
	RequestId  string
	Runtime    string
	Tag        string
	TokenId    string
}

type ChainElectionPacket struct {
	Type    string
	Key     string
	Meta    map[string]any
	Payload []byte
}

type ChainStakePacket struct {
	NodeId      string `json:"nodeId"`
	OwnerId     string `json:"ownerId"`
	Action      string `json:"action"`
	Amount      int64  `json:"amount"`
	Nonce       uint64 `json:"nonce"`
	LockSeconds int64  `json:"lockSeconds"`
	Reason      string `json:"reason"`
	Timestamp   int64  `json:"timestamp"`
}

type StakingNodeState struct {
	NodeId         string `json:"nodeId"`
	OwnerId        string `json:"ownerId"`
	BondedStake    int64  `json:"bondedStake"`
	PendingUnbond  int64  `json:"pendingUnbond"`
	UnbondUnlockAt int64  `json:"unbondUnlockAt"`
	Nonce          uint64 `json:"nonce"`
	LastUpdatedAt  int64  `json:"lastUpdatedAt"`
}

type ValidatorSetRecord struct {
	RoundId      string   `json:"roundId"`
	Validators   []string `json:"validators"`
	TotalBonded  int64    `json:"totalBonded"`
	SelectedAt   int64    `json:"selectedAt"`
	EligibleNode int      `json:"eligibleNode"`
}

type ElectionRound struct {
	Id                 string            `json:"id"`
	CommitSeed         string            `json:"commitSeed"`
	Phase              string            `json:"phase"`
	StartAt            int64             `json:"startAt"`
	CommitDeadline     int64             `json:"commitDeadline"`
	RevealDeadline     int64             `json:"revealDeadline"`
	FinalizedAt        int64             `json:"finalizedAt"`
	Commits            map[string]string `json:"commits"`
	Reveals            map[string]string `json:"reveals"`
	CommitParticipants map[string]bool   `json:"commitParticipants"`
	SelectedValidators []string          `json:"selectedValidators"`
}

type Election struct {
	MyNum        string
	Participants map[string]bool
	Commits      map[string][]byte
	Reveals      map[string]string
}

type ChainCallback struct {
	Fn        func([]byte, int, error)
	Executors map[string]bool
	Responses map[string]string
	Tag       string
}

type MessageCallback struct {
	Id string
	Fn func(string, []byte)
}

type Effects struct {
	DbUpdates []update.Update `json:"dbUpdates"`
}
