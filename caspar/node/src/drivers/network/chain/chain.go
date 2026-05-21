package chain

import (
	"crypto/tls"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	"kasper/src/drivers/network/chain/babble"
	"kasper/src/drivers/network/chain/config"
	"kasper/src/drivers/network/chain/crypto/keys"
	"kasper/src/drivers/network/chain/hashgraph"
	"kasper/src/drivers/network/chain/net"
	"kasper/src/drivers/network/chain/net/signal/wamp"
	"kasper/src/drivers/network/chain/node/state"
	"kasper/src/drivers/network/chain/peers"
	"kasper/src/drivers/network/chain/proxy"
	"kasper/src/drivers/network/chain/proxy/inmem"
	"kasper/src/drivers/network/chain/service"
	"kasper/src/shell/api/model"
	"kasper/src/shell/utils/future"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/google/uuid"
	cmap "github.com/orcaman/concurrent-map/v2"
)

type WorkChain struct {
	Id          string
	StoreId     string
	blockchain  *Blockchain
	mainLedger  *babble.Babble
	mainProxy   *inmem.InmemProxy
	shardChains cmap.ConcurrentMap[string, *ShardChain]
}

type ShardChain struct {
	Id          string
	shardLedger *babble.Babble
	shardProxy  *inmem.InmemProxy
}

type Blockchain struct {
	app         core.ICore
	chains      cmap.ConcurrentMap[string, *WorkChain]
	pipeline    func([][]byte, func([]byte)) []string
	trans       net.Transport
	service     *service.Service
	storageRoot string
}

type CLIConfig struct {
	Babble     config.Config `mapstructure:",squash"`
	ProxyAddr  string        `mapstructure:"proxy-listen"`
	ClientAddr string        `mapstructure:"client-connect"`
}

func initChainService(config *config.Config) *service.Service {
	if !config.NoService {
		service := service.NewService(config.ServiceAddr, config.Logger())
		future.Async(func() {
			service.Serve()
		}, false)
		return service
	}
	return nil
}

func initTransport(config *config.Config) (net.Transport, error) {
	// Leave nil transport if maintenance-mode is activated
	if config.MaintenanceMode {
		return nil, nil
	}

	if config.WebRTC {
		signal, err := wamp.NewClient(
			config.SignalAddr,
			config.SignalRealm,
			keys.PublicKeyHex(&config.Key.PublicKey),
			config.CertFile(),
			config.SignalSkipVerify,
			config.TCPTimeout,
			config.Logger().WithField("component", "webrtc-signal"),
		)

		if err != nil {
			return nil, err
		}

		webRTCTransport, err := net.NewWebRTCTransport(
			signal,
			config.ICEServers(),
			config.MaxPool,
			config.TCPTimeout,
			config.JoinTimeout,
			config.Logger().WithField("component", "webrtc-transport"),
		)

		if err != nil {
			return nil, err
		}

		return webRTCTransport, nil
	} else {
		tcpTransport, err := net.NewTCPTransport(
			config.BindAddr,
			config.AdvertiseAddr,
			config.MaxPool,
			config.TCPTimeout,
			config.JoinTimeout,
			config.Logger(),
		)

		if err != nil {
			return nil, err
		}

		return tcpTransport, nil
	}
}

func (b *Blockchain) createNewWorkChain(chainId string, storeId string, persist bool) *WorkChain {
	if existing, ok := b.chains.Get(chainId); ok {
		return existing
	}
	wchain := &WorkChain{Id: chainId, StoreId: storeId, mainLedger: nil, mainProxy: nil, shardChains: cmap.New[*ShardChain](), blockchain: b}
	b.chains.Set(chainId, wchain)
	mainShardChain := wchain.createNewShardChain("shard-main", false, []string{}, persist)
	wchain.mainLedger = mainShardChain.shardLedger
	wchain.mainProxy = mainShardChain.shardProxy
	if persist {
		b.app.ModifyState(false, func(trx trx.ITrx) error {
			model.Chain{Id: chainId, StoreId: storeId}.Push(trx)
			return nil
		})
	}
	return wchain
}

func (w *WorkChain) createNewShardChain(chainId string, created bool, peersArr []string, persist bool) *ShardChain {
	if existing, ok := w.shardChains.Get(chainId); ok {
		return existing
	}
	handler := &HgHandler{
		Chain: w,
	}
	proxy := inmem.NewInmemProxy(handler, nil)

	dataDir := w.blockchain.storageRoot + "/chains/" + w.Id + "/" + chainId
	os.MkdirAll(dataDir, os.ModePerm)

	peersListMode := "0"

	if created {
		mainChain, _ := w.blockchain.chains.Get("main")
		peersList := []*peers.Peer{}
		log.Println("shard peers", peersArr)
		log.Println("inspecting all nodes...")
		for _, peer := range mainChain.mainLedger.Peers.Peers {
			if slices.Contains(peersArr, strings.Split(peer.NetAddr, ":")[0]) {
				log.Println("node matched", peer)
				peersList = append(peersList, peer)
			}
		}
		log.Println("inspection finished.")
		peerset := peers.PeerSet{Peers: peersList}
		peersStr, _ := peerset.Marshal()
		peersFile, _ := os.OpenFile(dataDir+"/peers.json", os.O_WRONLY|os.O_CREATE, 0600)
		defer peersFile.Close()
		peersFile.Write(peersStr)
		peersListMode = "1"
	} else if os.Getenv("IS_HEAD") == "true" {
		peersListMode = "2"
	} else {
		peersListMode = "3"
	}

	cmd := exec.Command("bash", "/app/scripts/shardchain.sh", w.blockchain.storageRoot, w.Id, chainId, peersListMode)
	err := cmd.Run()
	if err != nil {
		log.Println(err)
	}

	config := config.NewDefaultConfig(os.Getenv("IPADDR") + ":" + os.Getenv("BLOCKCHAIN_API_PORT"))
	config.DataDir = dataDir
	config.Proxy = proxy
	engine := babble.NewBabble(config)
	if err := engine.Init(w.blockchain.trans, w.Id, chainId, func(origin string) {

	}); err != nil {
		panic(err)
	}
	shardChain := &ShardChain{Id: chainId, shardLedger: engine, shardProxy: proxy}
	w.shardChains.Set(chainId, shardChain)
	w.blockchain.service.RegisterNode(w.Id, chainId, engine.Node)
	if persist {
		w.blockchain.app.ModifyState(false, func(trx trx.ITrx) error {
			model.ChainShard{Id: chainId, WorkChainId: w.Id}.Push(trx)
			return nil
		})
	}
	future.Async(func() {
		engine.Run()
	}, false)
	return shardChain
}

func (b *Blockchain) RestoreFromStorage() { b.restoreChainsFromStorage() }

func (b *Blockchain) restoreChainsFromStorage() {
	restored := 0
	b.app.ModifyState(true, func(trx trx.ITrx) error {
		chains, err := model.Chain{}.All(trx, -1, -1, map[string]string{})
		if err != nil {
			return err
		}
		shards, err := model.ChainShard{}.All(trx, -1, -1, map[string]string{})
		if err != nil {
			return err
		}
		shardsByChain := map[string][]string{}
		for _, shard := range shards {
			shardsByChain[shard.WorkChainId] = append(shardsByChain[shard.WorkChainId], shard.Id)
		}
		for _, ch := range chains {
			wchain := b.createNewWorkChain(ch.Id, ch.StoreId, false)
			for _, shardId := range shardsByChain[ch.Id] {
				if shardId == "shard-main" {
					continue
				}
				wchain.createNewShardChain(shardId, false, []string{}, false)
			}
			restored++
		}
		return nil
	})
	if restored == 0 {
		b.createNewWorkChain("main", "", true)
	}
}

func NewChain(core core.ICore, storageRoot string) *Blockchain {
	blockchain := &Blockchain{
		app:         core,
		chains:      cmap.New[*WorkChain](),
		storageRoot: storageRoot,
		trans:       nil,
		service:     nil,
		pipeline:    nil,
	}
	config := config.NewDefaultConfig(os.Getenv("IPADDR") + ":" + os.Getenv("BLOCKCHAIN_API_PORT"))
	trans, err := initTransport(config)
	if err != nil {
		panic(err)
	}
	blockchain.trans = trans
	service := initChainService(config)
	blockchain.service = service
	// Storage-backed chain restore is deferred to RestoreFromStorage(),
	// which the caller invokes after Core.tools has been wired up.
	// Calling ModifyState here would dereference c.tools (nil during driver
	// construction) and panic.
	return blockchain
}

func (b *Blockchain) Listen(port int, tlsConfig *tls.Config) {
	for wchain := range b.chains.IterBuffered() {
		for schain := range wchain.Val.shardChains.IterBuffered() {
			future.Async(func() {
				schain.Val.shardLedger.Run()
			}, false)
		}
	}
}

func (b *Blockchain) Close() {
	for wchain := range b.chains.IterBuffered() {
		for schain := range wchain.Val.shardChains.IterBuffered() {
			schain.Val.shardLedger.Node.Leave()
		}
	}
}

func (c *Blockchain) RegisterPipeline(pipeline func([][]byte, func([]byte)) []string) {
	c.pipeline = pipeline
}

func (c *Blockchain) Peers() []string {
	peers := []string{}
	mainWorkChain, _ := c.chains.Get("main")
	mainShardChain, _ := mainWorkChain.shardChains.Get("shard-main")
	for _, peer := range mainShardChain.shardLedger.Peers.Peers {
		peers = append(peers, strings.Split(peer.NetAddr, ":")[0])
	}
	return peers
}

func (c *Blockchain) SubmitTrx(chainId string, machineId string, typ string, payload []byte) {
	_ = typ
	workChain, found := c.chains.Get(chainId)
	if !found {
		log.Println("work chain not found", chainId)
		return
	}
	targetShardId := "shard-main"
	if machineId != "" {
		c.app.ModifyState(true, func(trx trx.ITrx) error {
			vm := model.Program{MachineId: machineId}.Pull(trx)
			if vm.AppId == "" {
				return nil
			}
			app := model.Machine{Id: vm.AppId}.Pull(trx)
			if app.ChainId == chainId && app.ShardChainId != "" {
				targetShardId = app.ShardChainId
			}
			return nil
		})
	}
	targetShardChain, found := workChain.shardChains.Get(targetShardId)
	if !found {
		log.Println("target shard chain not found", chainId, targetShardId)
		return
	}
	targetShardChain.shardProxy.SubmitTx(payload)
}

func (c *Blockchain) NotifyNewMachineCreated(chainId string, machineId string) {
	// mainWorkChain, found := c.chains.Get(chainId)
	// if found {

	// }
}

func (c *Blockchain) CreateTempChain(storeId string) string {
	return c.createNewWorkChain(uuid.NewString(), storeId, true).Id
}

func (c *Blockchain) CreateWorkChain(storeId string) string {
	return c.createNewWorkChain(uuid.NewString(), storeId, true).Id
}

func (c *Blockchain) CreateShardChain(chainId string, shardChainId string, peers []string) string {
	workChain, found := c.chains.Get(chainId)
	if !found {
		return ""
	}
	if shardChainId == "" {
		shardChainId = "shard-" + uuid.NewString()
	}
	if _, exists := workChain.shardChains.Get(shardChainId); exists {
		return shardChainId
	}
	return workChain.createNewShardChain(shardChainId, true, peers, true).Id
}

func (c *Blockchain) UserOwnsOrigin(userId string, origin string) bool {
	return true
}

func (c *Blockchain) GetNodeOwnerId(origin string) string {
	return ""
}

type HgHandler struct {
	State state.State
	Chain *WorkChain
}

func (p *HgHandler) CommitHandler(block hashgraph.Block) (proxy.CommitResponse, error) {
	p.Chain.blockchain.pipeline(block.Transactions(), func(insiderTrx []byte) {

	})

	receipts := []hashgraph.InternalTransactionReceipt{}
	for _, it := range block.InternalTransactions() {
		receipts = append(receipts, it.AsAccepted())
	}
	response := proxy.CommitResponse{
		StateHash:                   []byte("statehash"),
		InternalTransactionReceipts: receipts,
	}
	return response, nil
}

func (p *HgHandler) StateChangeHandler(state state.State) error {
	p.State = state
	return nil
}

func (p *HgHandler) SnapshotHandler(blockIndex int) ([]byte, error) {
	return []byte("statehash"), nil
}

func (p *HgHandler) RestoreHandler(snapshot []byte) ([]byte, error) {
	return []byte("statehash"), nil
}
