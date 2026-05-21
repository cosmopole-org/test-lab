package telemetry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
)

type Server struct {
	db             *badger.DB
	startedAt      time.Time
	chainPort      string
	federationPort string
	clientTCPPort  string
	clientWSPort   string
	entityPort     string
	vmPort         string
	telemetryPort  string
	mu             sync.Mutex
}

type Snapshot struct {
	Timestamp       string                 `json:"timestamp"`
	UptimeSec       int64                  `json:"uptime_sec"`
	Node            map[string]interface{} `json:"node"`
	Chain           map[string]interface{} `json:"chain"`
	Federation      map[string]interface{} `json:"federation"`
	Clients         map[string]interface{} `json:"clients"`
	ProtocolTraffic map[string]interface{} `json:"protocol_traffic"`
	VMs             map[string]interface{} `json:"vms"`
	Machines        map[string]interface{} `json:"machines"`
	Costs           map[string]interface{} `json:"costs"`
	Transactions    map[string]interface{} `json:"transactions"`
	Packets         map[string]interface{} `json:"packets"`
	Messages        map[string]interface{} `json:"messages"`
	Creatures       map[string]interface{} `json:"creatures"`
	Validators      map[string]interface{} `json:"validators"`
	Staking         map[string]interface{} `json:"staking"`
	Election        map[string]interface{} `json:"election"`
}

func StartFromEnv() error {
	dbPath := os.Getenv("TELEMETRY_DB_PATH")
	if strings.TrimSpace(dbPath) == "" {
		root := os.Getenv("STORAGE_ROOT_PATH")
		if root == "" {
			root = "."
		}
		dbPath = filepath.Join(root, "telemetry-badger")
	}
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		return err
	}

	opts := badger.DefaultOptions(dbPath).WithLogger(nil)
	db, err := badger.Open(opts)
	if err != nil {
		return err
	}

	s := &Server{
		db:             db,
		startedAt:      time.Now(),
		chainPort:      os.Getenv("BLOCKCHAIN_API_PORT"),
		federationPort: os.Getenv("FEDERATION_API_PORT"),
		clientTCPPort:  os.Getenv("CLIENT_TCP_API_PORT"),
		clientWSPort:   os.Getenv("CLIENT_WS_API_PORT"),
		entityPort:     os.Getenv("ENTITY_API_PORT"),
		vmPort:         os.Getenv("VM_API_PORT"),
		telemetryPort:  envOr("TELEMETRY_API_PORT", "9099"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/telemetry/snapshot", s.handleSnapshot)
	mux.HandleFunc("/telemetry/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	go func() {
		_ = http.ListenAndServe("0.0.0.0:"+s.telemetryPort, mux)
	}()
	return nil
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap, err := s.cachedOrCollect()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

func (s *Server) cachedOrCollect() (*Snapshot, error) {
	var cached Snapshot
	fresh := false

	_ = s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("latest_snapshot"))
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			if err := json.Unmarshal(v, &cached); err != nil {
				return err
			}
			t, err := time.Parse(time.RFC3339, cached.Timestamp)
			if err == nil && time.Since(t) < 2*time.Second {
				fresh = true
			}
			return nil
		})
	})
	if fresh {
		return &cached, nil
	}

	snap := s.collect()
	payload, _ := json.Marshal(snap)
	_ = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("latest_snapshot"), payload)
	})
	return &snap, nil
}

func (s *Server) collect() Snapshot {
	now := time.Now().UTC()
	chainStats, _ := fetchJSON("http://127.0.0.1:" + s.chainPort + "/stats")
	peers, _ := fetchJSON("http://127.0.0.1:" + s.chainPort + "/peers")

	return Snapshot{
		Timestamp: now.Format(time.RFC3339),
		UptimeSec: int64(time.Since(s.startedAt).Seconds()),
		Node: map[string]interface{}{
			"origin":         os.Getenv("ORIGIN"),
			"telemetry_port": s.telemetryPort,
			"entity_port":    s.entityPort,
			"vm_port":        s.vmPort,
		},
		Chain: map[string]interface{}{
			"port":  s.chainPort,
			"stats": chainStats,
			"peers": peers,
		},
		Federation:      map[string]interface{}{"port": s.federationPort, "status": "running"},
		Clients:         map[string]interface{}{"tcp_port": s.clientTCPPort, "ws_port": s.clientWSPort},
		ProtocolTraffic: map[string]interface{}{"rx_bytes": 0, "tx_bytes": 0, "io_details": "attach protocol counters provider"},
		VMs:             map[string]interface{}{"running_count": 0, "details": []interface{}{}, "traffic": map[string]interface{}{"rx": 0, "tx": 0}},
		Machines:        map[string]interface{}{"running_count": 0, "details": []interface{}{}},
		Costs:           map[string]interface{}{"total_execution_cost": 0, "recent_task_costs": []interface{}{}},
		Transactions:    map[string]interface{}{"recent_processed": []interface{}{}, "count": 0},
		Packets:         map[string]interface{}{"recent": []interface{}{}, "count": 0},
		Messages:        map[string]interface{}{"recent_global_chain": []interface{}{}, "count": 0},
		Creatures:       map[string]interface{}{"onchain_realtime": []interface{}{}},
		Validators:      map[string]interface{}{"details": []interface{}{}, "count": 0},
		Staking:         map[string]interface{}{"total_staked": 0, "stats": map[string]interface{}{}},
		Election:        map[string]interface{}{"round": 0, "status": "unknown"},
	}
}

func fetchJSON(url string) (interface{}, error) {
	if strings.TrimSpace(url) == "" || strings.Contains(url, "127.0.0.1:/") {
		return nil, fmt.Errorf("invalid url")
	}
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v interface{}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func envOr(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	if _, err := strconv.Atoi(v); err != nil {
		return def
	}
	return v
}
