package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
)

type StorageMetrics struct {
	DatabaseUsedBytes int64  `json:"database_used_bytes"`
	DiskTotalBytes    uint64 `json:"disk_total_bytes"`
	DiskUsedBytes     uint64 `json:"disk_used_bytes"`
	DiskFreeBytes     uint64 `json:"disk_free_bytes"`
}

type NodeDetail struct {
	Address     string         `json:"address"`
	Status      string         `json:"status"`
	TotalChunks int            `json:"total_chunks"`
	Storage     StorageMetrics `json:"storage"`
}

type BootstrapDetail struct {
	Address string         `json:"address"`
	Status  string         `json:"status"`
	Storage StorageMetrics `json:"storage"`
}

type NetworkOverview struct {
	TotalFiles             int             `json:"total_files_secured"`
	ActiveNodesCount       int             `json:"active_nodes_count"`
	TotalChunksDistributed int             `json:"total_chunks_distributed"`
	AggregatedNodeStorage  StorageMetrics  `json:"aggregated_node_storage"`
	Bootstrap              BootstrapDetail `json:"bootstrap_node"`
	Nodes                  []NodeDetail    `json:"nodes_detail"`
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

func GetNetworkOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET allowed", http.StatusMethodNotAllowed)
		return
	}

	if len(config.BootStrapServers) == 0 {
		http.Error(w, "No bootstrap servers configured", http.StatusInternalServerError)
		return
	}

	var bootResp *http.Response
	var err error
	var targetBootstrap string
	var success bool

	perm := rand.Perm(len(config.BootStrapServers))
	for _, i := range perm {
		targetBootstrap = config.BootStrapServers[i]
		bootResp, err = httpClient.Get(fmt.Sprintf("http://%s/metrics", targetBootstrap))
		if err == nil && bootResp.StatusCode == 200 {
			success = true
			break
		}
	}

	if !success {
		http.Error(w, "Bootstrap metrics unavailable (Check if /metrics route exists on Bootstrap)", http.StatusServiceUnavailable)
		return
	}
	defer bootResp.Body.Close()

	var bootData struct {
		Status  string `json:"status"`
		Network struct {
			ActiveNodes   int      `json:"active_nodes"`
			TotalFiles    int      `json:"total_files"`
			NodeAddresses []string `json:"node_addresses"`
		} `json:"network"`
		Storage StorageMetrics `json:"storage"`
	}
	json.NewDecoder(bootResp.Body).Decode(&bootData)

	var wg sync.WaitGroup
	var mu sync.Mutex

	overview := NetworkOverview{
		TotalFiles:       bootData.Network.TotalFiles,
		ActiveNodesCount: bootData.Network.ActiveNodes,
		Bootstrap: BootstrapDetail{
			Address: targetBootstrap,
			Status:  bootData.Status,
			Storage: bootData.Storage,
		},
		Nodes: make([]NodeDetail, 0),
	}

	for _, nodeAddr := range bootData.Network.NodeAddresses {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()

			detail := NodeDetail{Address: addr, Status: "offline"}

			nResp, nErr := httpClient.Get(fmt.Sprintf("http://%s/metrics", addr))
			if nErr == nil {
				defer nResp.Body.Close()

				var nData struct {
					Status string `json:"status"`
					Data   struct {
						TotalChunks int `json:"total_chunks"`
					} `json:"data"`
					Storage StorageMetrics `json:"storage"`
				}

				if json.NewDecoder(nResp.Body).Decode(&nData) == nil {
					detail.Status = nData.Status
					detail.TotalChunks = nData.Data.TotalChunks
					detail.Storage = nData.Storage
				}
			}

			mu.Lock()
			overview.Nodes = append(overview.Nodes, detail)

			overview.TotalChunksDistributed += detail.TotalChunks

			overview.AggregatedNodeStorage.DatabaseUsedBytes += detail.Storage.DatabaseUsedBytes
			overview.AggregatedNodeStorage.DiskTotalBytes += detail.Storage.DiskTotalBytes
			overview.AggregatedNodeStorage.DiskUsedBytes += detail.Storage.DiskUsedBytes
			overview.AggregatedNodeStorage.DiskFreeBytes += detail.Storage.DiskFreeBytes
			mu.Unlock()

		}(nodeAddr)
	}

	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(overview)
}

func main() {
	config.InitHttpClient()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	bootstrapServersEnv := os.Getenv("BOOTSTRAP_SERVERS")
	if bootstrapServersEnv != "" {
		config.BootStrapServers = strings.Split(bootstrapServersEnv, ",")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/network/overview", GetNetworkOverview)

	log.Printf("[Explorer] Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Explorer failed: %v", err)
	}
}
