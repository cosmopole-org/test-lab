package globe

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"kasper/src/abstract/models/chain"
	"kasper/src/abstract/models/update"
	"kasper/src/shell/utils/crypto"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

type Globe struct {
	lock                sync.Mutex
	nodeID              string
	voterID             string
	maxValidatorCount   int
	electionCommitSecs  int64
	electionRevealSecs  int64
	peersFn             func() []string
	signPacketFn        func([]byte) string
	submitChainPacketFn func(chainId string, op any)
	setChainCallbackFn  func(string, *chain.ChainCallback)
	setMessageCbFn      func(string, *chain.MessageCallback)

	nodeStakes       map[string]int64
	nodeOwners       map[string]string
	nodeStates       map[string]*chain.StakingNodeState
	electionRound    *chain.ElectionRound
	lastElectionHour string
	totalBondedStake int64
	validatorHistory map[string]chain.ValidatorSetRecord
}

const (
	StakeActionBond   = "bond"
	StakeActionUnbond = "unbond"
	StakeActionSlash  = "slash"
	MinValidatorStake = int64(1_000)
	MaxValidatorStake = int64(1_000_000_000_000)
	UnbondingSeconds  = int64(24 * 60 * 60)
)

func NewGlobe(nodeID string, voterID string, peersFn func() []string, signPacketFn func([]byte) string, submitChainPacketFn func(chainId string, op any), setChainCallbackFn func(string, *chain.ChainCallback), setMessageCbFn func(string, *chain.MessageCallback), maxValidatorCount int, electionCommitSecs int64, electionRevealSecs int64) *Globe {
	return &Globe{
		nodeID:              nodeID,
		voterID:             voterID,
		peersFn:             peersFn,
		signPacketFn:        signPacketFn,
		submitChainPacketFn: submitChainPacketFn,
		setChainCallbackFn:  setChainCallbackFn,
		setMessageCbFn:      setMessageCbFn,
		maxValidatorCount:   maxValidatorCount,
		electionCommitSecs:  electionCommitSecs,
		electionRevealSecs:  electionRevealSecs,
		nodeStakes:          map[string]int64{},
		nodeOwners:          map[string]string{},
		nodeStates:          map[string]*chain.StakingNodeState{},
		electionRound:       nil,
		lastElectionHour:    "",
		totalBondedStake:    0,
		validatorHistory:    map[string]chain.ValidatorSetRecord{},
	}
}

func (g *Globe) SendBaseRequestOnChain(key string, payload []byte, signature string, userId string, tag string, callback func([]byte, int, error)) {
	callbackId := crypto.SecureUniqueString()
	g.setChainCallbackFn(callbackId, &chain.ChainCallback{Tag: tag, Fn: callback, Executors: map[string]bool{}, Responses: map[string]string{}})
	g.submitChainPacketFn("main", chain.ChainBaseRequest{Tag: tag, Signatures: []string{g.signPacketFn(payload), signature}, Submitter: g.nodeID, RequestId: callbackId, Author: "user::" + userId, Key: key, Payload: payload})
}

func (g *Globe) SendTypedMessageOnChain(chainId string, key string, messageType string, payload []byte, signature string, userId string, receivers map[string]map[string]bool, replyTo string, storeId string, pay *chain.ChainPayPacket, callback func(string, []byte)) {
	callbackId := ""
	if callback != nil {
		callbackId = crypto.SecureUniqueString()
		g.setMessageCbFn(callbackId, &chain.MessageCallback{Id: callbackId, Fn: callback})
	}
	if chainId == "" {
		chainId = "main"
	}
	g.submitChainPacketFn(chainId, chain.ChainMessage{Key: key, MessageType: messageType, ReplyTo: replyTo, StoreId: storeId, Pay: pay, Recievers: receivers, Signatures: []string{g.signPacketFn(payload), signature}, Submitter: g.nodeID, RequestId: callbackId, Author: "user::" + userId, Payload: payload})
}

func (g *Globe) ExecBaseResponseOnChain(callbackId string, packet []byte, signature string, resCode int, e string, updates []update.Update, tag string, toUserId string) {
	sort.Slice(updates, func(i, j int) bool {
		return (updates[i].Typ + ":" + updates[i].Key) < (updates[j].Typ + ":" + updates[j].Key)
	})
	g.submitChainPacketFn("main", chain.ChainResponse{ToUserId: toUserId, Tag: tag, Signature: signature, Executor: g.nodeID, RequestId: callbackId, ResCode: resCode, Err: e, Payload: packet, Effects: chain.Effects{DbUpdates: updates}})
}

func (g *Globe) Handle(typ string, trxPayload []byte) bool {
	switch typ {
	case "stake":
		return g.handleStakePacketPayload(trxPayload)
	case "election":
		return g.handleElectionPacketPayload(trxPayload)
	default:
		return false
	}
}

func (g *Globe) StakeNodeOwner(nodeId string, ownerId string, amount int64) {
	if nodeId == "" || ownerId == "" || amount < 0 {
		return
	}
	g.lock.Lock()
	nextNonce := uint64(1)
	if state, ok := g.nodeStates[nodeId]; ok {
		nextNonce = state.Nonce + 1
	}
	g.lock.Unlock()
	g.submitChainPacketFn("main", chain.ChainStakePacket{
		NodeId:    nodeId,
		OwnerId:   ownerId,
		Action:    StakeActionBond,
		Amount:    amount,
		Nonce:     nextNonce,
		Timestamp: time.Now().Unix(),
	})
}

func (g *Globe) TryStartScheduledElection(now time.Time) {
	g.lock.Lock()
	defer g.lock.Unlock()

	utcNow := now.UTC()
	hourKey := utcNow.Format("2006-01-02T15")
	if g.lastElectionHour == hourKey {
		return
	}
	if utcNow.Minute() != 0 || utcNow.Second() > 2 {
		return
	}
	g.lastElectionHour = hourKey

	roundId := utcNow.Format("2006-01-02T15")
	commitSeed := crypto.SecureUniqueString()
	g.submitElectionPacket(map[string]any{
		"phase":      "start-round",
		"roundId":    roundId,
		"voter":      g.voterID,
		"commitSeed": commitSeed,
	}, []byte("{}"))

	go func() {
		time.Sleep(time.Duration(g.electionCommitSecs) * time.Second)
		g.submitElectionPacket(map[string]any{
			"phase":   "start-reveal",
			"roundId": roundId,
		}, []byte("{}"))
		time.Sleep(time.Duration(g.electionRevealSecs) * time.Second)
		g.submitElectionPacket(map[string]any{
			"phase":   "finalize",
			"roundId": roundId,
		}, []byte("{}"))
	}()
}

func (g *Globe) submitElectionPacket(meta map[string]any, payload []byte) {
	if payload == nil {
		payload = []byte("{}")
	}
	g.submitChainPacketFn("main", chain.ChainElectionPacket{
		Type:    "election",
		Key:     "choose-validator",
		Meta:    meta,
		Payload: payload,
	})
}

func (g *Globe) handleStakePacketPayload(trxPayload []byte) bool {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.settleMatureUnbonds(time.Now().Unix())
	packet := chain.ChainStakePacket{}
	if err := json.Unmarshal(trxPayload, &packet); err != nil {
		log.Println(err)
		return true
	}
	if packet.NodeId == "" || packet.OwnerId == "" || packet.Amount <= 0 {
		return true
	}
	if packet.Action == "" {
		packet.Action = StakeActionBond
	}
	state, ok := g.nodeStates[packet.NodeId]
	if !ok {
		state = &chain.StakingNodeState{NodeId: packet.NodeId, OwnerId: packet.OwnerId}
		g.nodeStates[packet.NodeId] = state
	}
	if state.OwnerId != "" && state.OwnerId != packet.OwnerId {
		return true
	}
	if packet.Nonce > 0 && packet.Nonce <= state.Nonce {
		return true
	}
	if packet.Nonce > 0 {
		state.Nonce = packet.Nonce
	}
	state.OwnerId = packet.OwnerId
	now := time.Now().Unix()
	switch packet.Action {
	case StakeActionBond:
		g.applyBond(state, packet.Amount, packet.LockSeconds, now)
	case StakeActionUnbond:
		g.applyUnbond(state, packet.Amount, now)
	case StakeActionSlash:
		g.applySlash(state, packet.Amount, now)
	default:
		return true
	}
	state.LastUpdatedAt = now
	g.nodeOwners[packet.NodeId] = state.OwnerId
	g.nodeStakes[packet.NodeId] = state.BondedStake
	return true
}

func (g *Globe) applyBond(state *chain.StakingNodeState, amount int64, lockSeconds int64, now int64) {
	if amount <= 0 {
		return
	}
	newBonded := state.BondedStake + amount
	if newBonded > MaxValidatorStake {
		newBonded = MaxValidatorStake
	}
	g.totalBondedStake += newBonded - state.BondedStake
	state.BondedStake = newBonded
	if lockSeconds > 0 {
		lockUntil := now + lockSeconds
		if lockUntil > state.UnbondUnlockAt {
			state.UnbondUnlockAt = lockUntil
		}
	}
}

func (g *Globe) applyUnbond(state *chain.StakingNodeState, amount int64, now int64) {
	if amount <= 0 || state.BondedStake <= 0 {
		return
	}
	if now < state.UnbondUnlockAt {
		return
	}
	if amount > state.BondedStake {
		amount = state.BondedStake
	}
	state.BondedStake -= amount
	state.PendingUnbond += amount
	state.UnbondUnlockAt = now + UnbondingSeconds
	g.totalBondedStake -= amount
	if g.totalBondedStake < 0 {
		g.totalBondedStake = 0
	}
}

func (g *Globe) applySlash(state *chain.StakingNodeState, amount int64, now int64) {
	if amount <= 0 {
		return
	}
	slashBonded := amount
	if slashBonded > state.BondedStake {
		slashBonded = state.BondedStake
	}
	state.BondedStake -= slashBonded
	g.totalBondedStake -= slashBonded
	if g.totalBondedStake < 0 {
		g.totalBondedStake = 0
	}
	remaining := amount - slashBonded
	if remaining > 0 {
		if remaining > state.PendingUnbond {
			remaining = state.PendingUnbond
		}
		state.PendingUnbond -= remaining
	}
	state.UnbondUnlockAt = now + UnbondingSeconds
}

func (g *Globe) settleMatureUnbonds(now int64) {
	for _, state := range g.nodeStates {
		if state.PendingUnbond > 0 && now >= state.UnbondUnlockAt {
			state.PendingUnbond = 0
		}
	}
}

func (g *Globe) handleElectionPacketPayload(trxPayload []byte) bool {
	g.lock.Lock()
	defer g.lock.Unlock()
	packet := chain.ChainElectionPacket{}
	if err := json.Unmarshal(trxPayload, &packet); err != nil {
		log.Println(err)
		return true
	}
	g.handleElectionPacket(packet)
	return true
}

func (g *Globe) electionCommit(roundId string, seed string) string {
	hash := sha256.Sum256([]byte(roundId + "::" + g.nodeID + "::" + seed))
	return hex.EncodeToString(hash[:])
}

func (g *Globe) weightedValidatorsFromSeed(seed string) []string {
	type candidate struct {
		nodeId string
		stake  int64
		score  uint64
	}
	peers := g.peersFn()
	if len(peers) == 0 {
		return []string{}
	}
	candidates := []candidate{}
	for _, nodeId := range peers {
		state, ok := g.nodeStates[nodeId]
		if !ok || state.BondedStake < MinValidatorStake {
			continue
		}
		weight := state.BondedStake
		hashed := sha256.Sum256([]byte(seed + "::" + nodeId))
		rawScore := binary.BigEndian.Uint64(hashed[:8])
		score := rawScore / uint64(weight)
		candidates = append(candidates, candidate{nodeId: nodeId, stake: state.BondedStake, score: score})
	}
	if len(candidates) == 0 {
		return []string{}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].stake > candidates[j].stake
		}
		return candidates[i].score < candidates[j].score
	})
	target := len(candidates) / 3
	if target < 1 {
		target = 1
	}
	if target > g.maxValidatorCount {
		target = g.maxValidatorCount
	}
	if target > len(candidates) {
		target = len(candidates)
	}
	validators := make([]string, 0, target)
	for i := 0; i < target; i++ {
		validators = append(validators, candidates[i].nodeId)
	}
	return validators
}

func (g *Globe) handleElectionPacket(packet chain.ChainElectionPacket) {
	phaseRaw, ok := packet.Meta["phase"]
	if !ok {
		return
	}
	phase, _ := phaseRaw.(string)
	now := time.Now().Unix()
	g.settleMatureUnbonds(now)
	switch phase {
	case "start-round":
		roundId, _ := packet.Meta["roundId"].(string)
		commitSeed, _ := packet.Meta["commitSeed"].(string)
		if roundId == "" {
			return
		}
		g.electionRound = &chain.ElectionRound{
			Id:                 roundId,
			CommitSeed:         commitSeed,
			Phase:              "commit",
			StartAt:            now,
			CommitDeadline:     now + g.electionCommitSecs,
			RevealDeadline:     now + g.electionCommitSecs + g.electionRevealSecs,
			Commits:            map[string]string{},
			Reveals:            map[string]string{},
			CommitParticipants: map[string]bool{},
		}
		mySeed := crypto.SecureUniqueString()
		commit := g.electionCommit(roundId, mySeed)
		g.electionRound.Commits[g.nodeID] = commit
		g.electionRound.Reveals[g.nodeID] = mySeed
		g.electionRound.CommitParticipants[g.nodeID] = true
		g.submitElectionPacket(map[string]any{
			"phase":   "commit",
			"roundId": roundId,
			"nodeId":  g.nodeID,
			"commit":  commit,
		}, []byte("{}"))
	case "commit":
		if g.electionRound == nil {
			return
		}
		roundId, _ := packet.Meta["roundId"].(string)
		nodeId, _ := packet.Meta["nodeId"].(string)
		commit, _ := packet.Meta["commit"].(string)
		if roundId != g.electionRound.Id || nodeId == "" || commit == "" {
			return
		}
		g.electionRound.Commits[nodeId] = commit
		g.electionRound.CommitParticipants[nodeId] = true
	case "start-reveal":
		if g.electionRound == nil {
			return
		}
		roundId, _ := packet.Meta["roundId"].(string)
		if roundId != g.electionRound.Id {
			return
		}
		g.electionRound.Phase = "reveal"
		mySeed := g.electionRound.Reveals[g.nodeID]
		if mySeed != "" {
			g.submitElectionPacket(map[string]any{
				"phase":   "reveal",
				"roundId": roundId,
				"nodeId":  g.nodeID,
				"seed":    mySeed,
			}, []byte("{}"))
		}
	case "reveal":
		if g.electionRound == nil {
			return
		}
		roundId, _ := packet.Meta["roundId"].(string)
		nodeId, _ := packet.Meta["nodeId"].(string)
		seed, _ := packet.Meta["seed"].(string)
		if roundId != g.electionRound.Id || nodeId == "" || seed == "" {
			return
		}
		expected := g.electionRound.Commits[nodeId]
		if expected == "" {
			return
		}
		if g.electionCommit(roundId, seed) != expected {
			return
		}
		g.electionRound.Reveals[nodeId] = seed
	case "finalize":
		if g.electionRound == nil {
			return
		}
		roundId, _ := packet.Meta["roundId"].(string)
		if roundId != g.electionRound.Id {
			return
		}
		revealSeedParts := []string{}
		for nodeId, seed := range g.electionRound.Reveals {
			revealSeedParts = append(revealSeedParts, nodeId+"="+seed)
		}
		sort.Strings(revealSeedParts)
		globalSeed := fmt.Sprintf("%s::%s", roundId, strings.Join(revealSeedParts, "|"))
		g.electionRound.SelectedValidators = g.weightedValidatorsFromSeed(globalSeed)
		g.electionRound.Phase = "finalized"
		g.electionRound.FinalizedAt = now
		g.validatorHistory[roundId] = chain.ValidatorSetRecord{
			RoundId:      roundId,
			Validators:   g.electionRound.SelectedValidators,
			TotalBonded:  g.totalBondedStake,
			SelectedAt:   now,
			EligibleNode: len(g.electionRound.SelectedValidators),
		}
		log.Println("election finalized", g.electionRound.Id, g.electionRound.SelectedValidators)
	}
}
