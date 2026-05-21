package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"sync"
)

type requestEnvelope struct {
	Type  string          `json:"type"`
	Data  string          `json:"data"`
	Store storeDescriptor `json:"store"`
	User  userDescriptor  `json:"user"`
}

type storeDescriptor struct {
	ID string `json:"id"`
}

type userDescriptor struct {
	ID string `json:"id"`
}

type onchainRequest struct {
	RequestID        string          `json:"requestId"`
	MachineID        string          `json:"machineId"`
	StoreID          string          `json:"storeId"`
	MasmPath         string          `json:"masmPath"`
	Inputs           []uint64        `json:"inputs"`
	Outputs          []uint64        `json:"outputs"`
	Proof            []uint8         `json:"proof"`
	UserID           string          `json:"userId"`
	UserSignature    string          `json:"userSignature"`
	ExecutionPayload json.RawMessage `json:"executionPayload"`
}

var (
	lock            sync.Mutex
	conn            net.Conn
	callbackCounter uint64
	nodeID          = getEnv("VERIFIABLE_NODE_ID", "node-unknown")
	nodeRole        = getEnv("VERIFIABLE_NODE_ROLE", "verifier")
	voteState       = newVoteBook()
	electionState   = newElectionBook()
	shardState      = newShardBook()
)

type proofSharedMessage struct {
	Type         string          `json:"type"`
	RequestID    string          `json:"requestId"`
	StoreID      string          `json:"storeId"`
	ExecutorNode string          `json:"executorNode"`
	MachineID    string          `json:"machineId"`
	MasmPath     string          `json:"masmPath"`
	Inputs       []uint64        `json:"inputs"`
	Outputs      []uint64        `json:"outputs"`
	Proof        []uint8         `json:"proof"`
	ProofDigest  string          `json:"proofDigest"`
	Result       json.RawMessage `json:"result"`
	Requester    string          `json:"requester"`
}

type verificationVote struct {
	Type          string `json:"type"`
	RequestID     string `json:"requestId"`
	StoreID       string `json:"storeId"`
	Requester     string `json:"requester"`
	VerifierNode  string `json:"verifierNode"`
	Vote          string `json:"vote"`
	Reason        string `json:"reason,omitempty"`
	ProofDigest   string `json:"proofDigest"`
	SecurityLevel uint32 `json:"securityLevel,omitempty"`
}

type voteBook struct {
	mu    sync.Mutex
	votes map[string]map[string]verificationVote
}

type validatorStake struct {
	Type   string `json:"type"`
	NodeID string `json:"nodeId"`
	Stake  uint64 `json:"stake"`
}

type validatorCommit struct {
	Type   string `json:"type"`
	Period int64  `json:"period"`
	NodeID string `json:"nodeId"`
	Hash   string `json:"hash"`
}

type validatorReveal struct {
	Type   string `json:"type"`
	Period int64  `json:"period"`
	NodeID string `json:"nodeId"`
	Nonce  string `json:"nonce"`
	Stake  uint64 `json:"stake"`
}

type electionTick struct {
	Type           string `json:"type"`
	Period         int64  `json:"period"`
	ValidatorSlots int    `json:"validatorSlots"`
}

type electionWinner struct {
	NodeID string  `json:"nodeId"`
	Stake  uint64  `json:"stake"`
	Score  float64 `json:"score"`
}

type electionBook struct {
	mu      sync.Mutex
	stakes  map[string]uint64
	commits map[int64]map[string]string
	reveals map[int64]map[string]validatorReveal
	elected map[int64][]electionWinner
}

type machineLoadReport struct {
	Type      string  `json:"type"`
	WorkChain string  `json:"workChain"`
	MachineID string  `json:"machineId"`
	Cost      float64 `json:"cost"`
}

type shardMachine struct {
	MachineID string  `json:"machineId"`
	Cost      float64 `json:"cost"`
}

type shardGroup struct {
	ShardID   string         `json:"shardId"`
	Machines  []shardMachine `json:"machines"`
	TotalCost float64        `json:"totalCost"`
}

type shardBook struct {
	mu           sync.Mutex
	targetCost   float64
	chainMachine map[string]map[string]float64
}

func newVoteBook() *voteBook {
	return &voteBook{votes: map[string]map[string]verificationVote{}}
}

func newElectionBook() *electionBook {
	return &electionBook{
		stakes:  map[string]uint64{},
		commits: map[int64]map[string]string{},
		reveals: map[int64]map[string]validatorReveal{},
		elected: map[int64][]electionWinner{},
	}
}

func newShardBook() *shardBook {
	return &shardBook{
		targetCost:   100.0,
		chainMachine: map[string]map[string]float64{},
	}
}

func (v *voteBook) upsertVote(vote verificationVote) (yes int, no int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.votes[vote.RequestID]; !ok {
		v.votes[vote.RequestID] = map[string]verificationVote{}
	}
	v.votes[vote.RequestID][vote.VerifierNode] = vote
	for _, current := range v.votes[vote.RequestID] {
		if current.Vote == "yes" {
			yes++
		} else if current.Vote == "no" {
			no++
		}
	}
	return yes, no
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func hashCommit(period int64, nodeID, nonce string) string {
	d := sha256.Sum256([]byte(strconv.FormatInt(period, 10) + ":" + nodeID + ":" + nonce))
	return hex.EncodeToString(d[:])
}

func writePacket(data []byte, withCallback bool) {
	lock.Lock()
	defer lock.Unlock()

	lenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBytes, uint32(len(data)))
	_, _ = conn.Write(lenBytes)

	cb := make([]byte, 8)
	if withCallback {
		callbackCounter++
		binary.LittleEndian.PutUint64(cb, callbackCounter)
	}
	_, _ = conn.Write(cb)
	_, _ = conn.Write(data)
}

func hostCall(key string, input map[string]any) {
	packet, _ := json.Marshal(map[string]any{"key": key, "input": input})
	writePacket(packet, true)
}

func chainDriverApi(action string, input map[string]any) {
	hostCall("chainDriverApi", map[string]any{
		"action": action,
		"input":  input,
	})
}

func signalStore(storeID string, userID string, payload map[string]any) {
	body, _ := json.Marshal(payload)
	hostCall("signalStore", map[string]any{
		"type":    "broadcast",
		"storeId": storeID,
		"userId":  userID,
		"data":    string(body),
	})
}

func signalStoreStruct(storeID, userID string, payload any) {
	body, _ := json.Marshal(payload)
	hostCall("signalStore", map[string]any{
		"type":    "broadcast",
		"storeId": storeID,
		"userId":  userID,
		"data":    string(body),
	})
}

func runVm(machineID, storeID, masmPath string, inputs []uint64) {
	payload, _ := json.Marshal(map[string]any{"inputs": inputs})
	hostCall("runVm", map[string]any{
		"machineId": machineID,
		"storeId":   storeID,
		"astPath":   masmPath,
		"runtime":   "docker",
		"input":     string(payload),
	})
}

func elpifyProof(masmPath string, inputs []uint64, outputs []uint64, proof []uint8) {
	hostCall("elpifyProof", map[string]any{
		"masmPath": masmPath,
		"inputs":   inputs,
		"outputs":  outputs,
		"proof":    proof,
	})
}

func verifyApproval(req onchainRequest) bool {
	if req.UserID == "" || req.UserSignature == "" || req.RequestID == "" {
		return false
	}
	digest := sha256.Sum256([]byte(req.UserID + ":" + req.RequestID))
	expected := base64.StdEncoding.EncodeToString(digest[:])
	return req.UserSignature == expected
}

func processOnchainRequest(req onchainRequest) {
	if nodeRole != "executor" {
		return
	}

	if !verifyApproval(req) {
		signalStore(req.StoreID, req.UserID, map[string]any{
			"type":      "onchainExecutionRejected",
			"requestId": req.RequestID,
			"reason":    "invalid user approval signature",
		})
		return
	}

	signalStore(req.StoreID, req.UserID, map[string]any{
		"type":      "onchainExecutionAccepted",
		"requestId": req.RequestID,
		"nodeId":    nodeID,
		"nodeRole":  nodeRole,
	})

	runVm(req.MachineID, req.StoreID, req.MasmPath, req.Inputs)

	proofDigest := sha256.Sum256(req.Proof)
	proofMessage := proofSharedMessage{
		Type:         "onchainExecutionProofShared",
		RequestID:    req.RequestID,
		StoreID:      req.StoreID,
		ExecutorNode: nodeID,
		MachineID:    req.MachineID,
		MasmPath:     req.MasmPath,
		Inputs:       req.Inputs,
		Outputs:      req.Outputs,
		Proof:        req.Proof,
		ProofDigest:  hex.EncodeToString(proofDigest[:]),
		Result:       req.ExecutionPayload,
		Requester:    req.UserID,
	}
	signalStoreStruct(req.StoreID, req.UserID, proofMessage)

	signalStore(req.StoreID, req.UserID, map[string]any{
		"type":      "executionResultToRequester",
		"requestId": req.RequestID,
		"result":    string(req.ExecutionPayload),
		"fromNode":  nodeID,
	})
}

func parseProofSharedPayload(env requestEnvelope) (proofSharedMessage, error) {
	msg := proofSharedMessage{}
	err := json.Unmarshal([]byte(env.Data), &msg)
	return msg, err
}

func processProofShared(msg proofSharedMessage) {
	if msg.ExecutorNode == nodeID {
		return
	}

	elpifyProof(msg.MasmPath, msg.Inputs, msg.Outputs, msg.Proof)
	vote := verificationVote{
		Type:         "onchainVerificationVote",
		RequestID:    msg.RequestID,
		StoreID:      msg.StoreID,
		Requester:    msg.Requester,
		VerifierNode: nodeID,
		Vote:         "yes",
		ProofDigest:  msg.ProofDigest,
	}

	// Base toolkit: mark no if proof payload is missing before host verification.
	if len(msg.Proof) == 0 {
		vote.Vote = "no"
		vote.Reason = "empty proof payload"
	}
	signalStoreStruct(msg.StoreID, msg.Requester, vote)
}

func processVerificationVote(vote verificationVote) {
	yes, no := voteState.upsertVote(vote)
	status := "pending"
	if yes > no {
		status = "accepted"
	} else if no > yes {
		status = "rejected"
	}

	signalStore(vote.StoreID, vote.Requester, map[string]any{
		"type":      "onchainVerificationTally",
		"requestId": vote.RequestID,
		"yesVotes":  yes,
		"noVotes":   no,
		"status":    status,
	})
}

func processValidatorStake(storeID string, msg validatorStake) {
	electionState.mu.Lock()
	defer electionState.mu.Unlock()
	electionState.stakes[msg.NodeID] = msg.Stake
	signalStore(storeID, msg.NodeID, map[string]any{
		"type":   "validatorStakeAccepted",
		"nodeId": msg.NodeID,
		"stake":  msg.Stake,
	})
}

func processValidatorCommit(storeID string, msg validatorCommit) {
	electionState.mu.Lock()
	defer electionState.mu.Unlock()
	if _, ok := electionState.commits[msg.Period]; !ok {
		electionState.commits[msg.Period] = map[string]string{}
	}
	electionState.commits[msg.Period][msg.NodeID] = msg.Hash
	signalStore(storeID, msg.NodeID, map[string]any{
		"type":   "validatorCommitAccepted",
		"period": msg.Period,
		"nodeId": msg.NodeID,
	})
}

func processValidatorReveal(storeID string, msg validatorReveal) {
	electionState.mu.Lock()
	defer electionState.mu.Unlock()
	commitHash := ""
	if cset, ok := electionState.commits[msg.Period]; ok {
		commitHash = cset[msg.NodeID]
	}
	expected := hashCommit(msg.Period, msg.NodeID, msg.Nonce)
	if commitHash == "" || commitHash != expected {
		signalStore(storeID, msg.NodeID, map[string]any{
			"type":   "validatorRevealRejected",
			"period": msg.Period,
			"nodeId": msg.NodeID,
			"reason": "commit/reveal mismatch",
		})
		return
	}
	if _, ok := electionState.reveals[msg.Period]; !ok {
		electionState.reveals[msg.Period] = map[string]validatorReveal{}
	}
	electionState.reveals[msg.Period][msg.NodeID] = msg
	electionState.stakes[msg.NodeID] = msg.Stake
}

func processElectionTick(storeID string, msg electionTick) {
	electionState.mu.Lock()
	defer electionState.mu.Unlock()
	reveals := electionState.reveals[msg.Period]
	if len(reveals) == 0 {
		return
	}

	nonces := make([]string, 0, len(reveals))
	for _, rv := range reveals {
		nonces = append(nonces, rv.Nonce)
	}
	sort.Strings(nonces)
	seedBytes := sha256.Sum256([]byte(bytes.Join(func() [][]byte {
		out := make([][]byte, 0, len(nonces))
		for _, n := range nonces {
			out = append(out, []byte(n))
		}
		return out
	}(), []byte("|"))))

	winners := make([]electionWinner, 0, len(reveals))
	for _, rv := range reveals {
		randHash := sha256.Sum256([]byte(rv.NodeID + ":" + hex.EncodeToString(seedBytes[:])))
		weight := float64(randHash[0])/255.0 + 1.0
		score := weight * float64(rv.Stake)
		winners = append(winners, electionWinner{
			NodeID: rv.NodeID,
			Stake:  rv.Stake,
			Score:  score,
		})
	}
	sort.Slice(winners, func(i, j int) bool { return winners[i].Score > winners[j].Score })
	slots := msg.ValidatorSlots
	if slots <= 0 {
		slots = 4
	}
	if slots > len(winners) {
		slots = len(winners)
	}
	winners = winners[:slots]
	electionState.elected[msg.Period] = winners

	signalStoreStruct(storeID, "", map[string]any{
		"type":    "onchainValidatorElectionResult",
		"period":  msg.Period,
		"winners": winners,
		"seed":    hex.EncodeToString(seedBytes[:]),
	})
}

func processMachineLoadReport(msg machineLoadReport) {
	shardState.mu.Lock()
	defer shardState.mu.Unlock()
	if _, ok := shardState.chainMachine[msg.WorkChain]; !ok {
		shardState.chainMachine[msg.WorkChain] = map[string]float64{}
	}
	shardState.chainMachine[msg.WorkChain][msg.MachineID] = msg.Cost
	rebalanceShards(msg.WorkChain)
}

func rebalanceShards(workChain string) {
	machineMap := shardState.chainMachine[workChain]
	if len(machineMap) == 0 {
		return
	}

	machines := make([]shardMachine, 0, len(machineMap))
	totalCost := 0.0
	for machineID, cost := range machineMap {
		machines = append(machines, shardMachine{MachineID: machineID, Cost: cost})
		totalCost += cost
	}
	sort.Slice(machines, func(i, j int) bool { return machines[i].Cost > machines[j].Cost })

	shardCount := int(math.Ceil(totalCost / shardState.targetCost))
	if shardCount < 1 {
		shardCount = 1
	}

	shards := make([]shardGroup, shardCount)
	for i := 0; i < shardCount; i++ {
		shards[i] = shardGroup{ShardID: workChain + "-shard-" + strconv.Itoa(i+1)}
	}

	for _, machine := range machines {
		lowest := 0
		for i := 1; i < len(shards); i++ {
			if shards[i].TotalCost < shards[lowest].TotalCost {
				lowest = i
			}
		}
		shards[lowest].Machines = append(shards[lowest].Machines, machine)
		shards[lowest].TotalCost += machine.Cost
	}

	// Broadcast sharding plan on chain and call chain-driver API hooks for create/update.
	signalStoreStruct(workChain, "", map[string]any{
		"type":   "onchainShardPlan",
		"chain":  workChain,
		"shards": shards,
	})
	for _, shard := range shards {
		chainDriverApi("upsertSubChain", map[string]any{
			"workChain": workChain,
			"shardId":   shard.ShardID,
			"machines":  shard.Machines,
			"totalCost": shard.TotalCost,
		})
	}
	chainDriverApi("rebalanceSubChains", map[string]any{
		"workChain":  workChain,
		"shardCount": shardCount,
	})
}

func processPacket(data []byte) {
	var env requestEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		log.Printf("failed to decode packet envelope: %v", err)
		return
	}

	payloadType := env.Type
	if env.Data != "" {
		var envelopeData map[string]any
		if err := json.Unmarshal([]byte(env.Data), &envelopeData); err == nil {
			if t, ok := envelopeData["type"].(string); ok && t != "" {
				payloadType = t
			}
		}
	}

	if payloadType != "onchainExecutionRequest" {
		if payloadType == "onchainExecutionProofShared" {
			msg, err := parseProofSharedPayload(env)
			if err != nil {
				log.Printf("failed to decode proof-shared payload: %v", err)
				return
			}
			if msg.StoreID == "" {
				msg.StoreID = env.Store.ID
			}
			if msg.Requester == "" {
				msg.Requester = env.User.ID
			}
			processProofShared(msg)
		}
		if payloadType == "onchainVerificationVote" {
			vote := verificationVote{}
			if err := json.Unmarshal([]byte(env.Data), &vote); err != nil {
				log.Printf("failed to decode verification vote: %v", err)
				return
			}
			if vote.StoreID == "" {
				vote.StoreID = env.Store.ID
			}
			if vote.Requester == "" {
				vote.Requester = env.User.ID
			}
			processVerificationVote(vote)
		}
		if payloadType == "validatorStakeAnnouncement" {
			stake := validatorStake{}
			if err := json.Unmarshal([]byte(env.Data), &stake); err == nil {
				processValidatorStake(env.Store.ID, stake)
			}
		}
		if payloadType == "validatorCommit" {
			commit := validatorCommit{}
			if err := json.Unmarshal([]byte(env.Data), &commit); err == nil {
				processValidatorCommit(env.Store.ID, commit)
			}
		}
		if payloadType == "validatorReveal" {
			reveal := validatorReveal{}
			if err := json.Unmarshal([]byte(env.Data), &reveal); err == nil {
				processValidatorReveal(env.Store.ID, reveal)
			}
		}
		if payloadType == "validatorElectionTick" {
			tick := electionTick{}
			if err := json.Unmarshal([]byte(env.Data), &tick); err == nil {
				processElectionTick(env.Store.ID, tick)
			}
		}
		if payloadType == "machineLoadReport" {
			report := machineLoadReport{}
			if err := json.Unmarshal([]byte(env.Data), &report); err == nil {
				if report.WorkChain == "" {
					report.WorkChain = env.Store.ID
				}
				processMachineLoadReport(report)
			}
		}
		return
	}

	var req onchainRequest
	if err := json.Unmarshal([]byte(env.Data), &req); err != nil {
		log.Printf("failed to decode onchain request payload: %v", err)
		return
	}
	if req.StoreID == "" {
		req.StoreID = env.Store.ID
	}
	if req.UserID == "" {
		req.UserID = env.User.ID
	}
	processOnchainRequest(req)
}

func main() {
	var err error
	conn, err = net.Dial("tcp", "10.10.0.3:8084")
	if err != nil {
		log.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for {
		var ln uint32
		if err := binary.Read(reader, binary.LittleEndian, &ln); err != nil {
			if err != io.EOF {
				log.Printf("read len err: %v", err)
			}
			os.Exit(0)
		}
		var callbackID uint64
		if err := binary.Read(reader, binary.LittleEndian, &callbackID); err != nil {
			if err != io.EOF {
				log.Printf("read callback err: %v", err)
			}
			os.Exit(0)
		}
		body := make([]byte, ln)
		if _, err := io.ReadFull(reader, body); err != nil {
			log.Printf("read body err: %v", err)
			os.Exit(0)
		}
		if callbackID == 0 {
			processPacket(body)
		}
	}
}
