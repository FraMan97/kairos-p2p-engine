package api

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/crypto"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/models"
	"github.com/FraMan97/kairos-p2p-engine/internal/service"
)

func SubsribeNode(w http.ResponseWriter, r *http.Request) {
	var subscription models.SubscriptionRequest
	json.NewDecoder(r.Body).Decode(&subscription)
	defer r.Body.Close()

	message, _ := json.Marshal(models.SubscriptionRequest{Address: subscription.Address, PublicKey: subscription.PublicKey})
	check, _ := crypto.VerifySignature(message, subscription.Signature, subscription.PublicKey)

	if check {
		payload, _ := json.Marshal(models.ActiveNodeRecord{PublicKey: subscription.PublicKey, Timestamp: time.Now().UnixNano()})
		database.PutData(config.DB, "active_nodes", subscription.Address, payload)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func SynchronizeData(w http.ResponseWriter, r *http.Request) {
	var receivedData models.SynchronizationRequest
	json.NewDecoder(r.Body).Decode(&receivedData)
	defer r.Body.Close()

	message, _ := json.Marshal(models.SynchronizationRequest{Address: receivedData.Address, PublicKey: receivedData.PublicKey, ActiveNodes: receivedData.ActiveNodes, FileManifests: receivedData.FileManifests})
	check, _ := crypto.VerifySignature(message, receivedData.Signature, receivedData.PublicKey)

	if check {
		activeNodes, _ := database.GetAllData(config.DB, "active_nodes")
		fileManifests, _ := database.GetAllData(config.DB, "manifests")

		dataToExchange := models.SynchronizationRequest{
			Address:       config.AdvertisedAddress + ":" + strconv.Itoa(config.Port),
			PublicKey:     config.PublicKey,
			ActiveNodes:   activeNodes,
			FileManifests: fileManifests,
		}

		jsonBytes, _ := json.Marshal(dataToExchange)
		signature, _ := crypto.SignMessage(jsonBytes)
		dataToExchange.Signature = signature

		jsonBytes, _ = json.Marshal(dataToExchange)
		go service.ProcessAlignment(dataToExchange, receivedData)
		w.Write(jsonBytes)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func RequestNodesForFileUploadAPI(w http.ResponseWriter, r *http.Request) {
	var request models.NodesForFileUploadRequest
	json.NewDecoder(r.Body).Decode(&request)
	defer r.Body.Close()

	message, _ := json.Marshal(models.NodesForFileUploadRequest{Address: request.Address, PublicKey: request.PublicKey, TotalChunks: request.TotalChunks, NodesPerChunk: request.NodesPerChunk})
	check, _ := crypto.VerifySignature(message, request.Signature, request.PublicKey)

	if check {
		allDBNodes, _ := database.GetAllKeys(config.DB, "active_nodes")
		var response []string
		nodesToPickup := int(math.Ceil(float64(request.TotalChunks) / float64(request.NodesPerChunk)))

		if len(allDBNodes) <= nodesToPickup {
			response = allDBNodes
		} else {
			response = allDBNodes[:nodesToPickup] // Simple slice for now
		}

		jsonBytes, _ := json.Marshal(response)
		w.Write(jsonBytes)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func InsertFileManifest(w http.ResponseWriter, r *http.Request) {
	var request models.FileManifestRequest
	json.NewDecoder(r.Body).Decode(&request)
	defer r.Body.Close()

	manifestBytes, _ := json.Marshal(request.Manifest)
	hashToVerify := sha256.Sum256(manifestBytes)

	check, _ := crypto.VerifySignature(hashToVerify[:], request.Signature, request.PublicKey)
	if check {
		database.PutData(config.DB, "manifests", request.Manifest.FileId, manifestBytes)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func DownloadFileManifest(w http.ResponseWriter, r *http.Request) {
	var request models.GetFileManifestRequest
	json.NewDecoder(r.Body).Decode(&request)
	defer r.Body.Close()

	message, _ := json.Marshal(models.GetFileManifestRequest{Address: request.Address, PublicKey: request.PublicKey, FileId: request.FileId})
	check, _ := crypto.VerifySignature(message, request.Signature, request.PublicKey)

	if check {
		dbData, _ := database.GetData(config.DB, "manifests", request.FileId)
		w.Write(dbData)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func DeleteFileManifest(w http.ResponseWriter, r *http.Request) {
	var request models.GetFileManifestRequest
	json.NewDecoder(r.Body).Decode(&request)
	defer r.Body.Close()

	message, _ := json.Marshal(models.GetFileManifestRequest{Address: request.Address, PublicKey: request.PublicKey, FileId: request.FileId})
	check, _ := crypto.VerifySignature(message, request.Signature, request.PublicKey)

	if check {
		database.DeleteKey(config.DB, "manifests", request.FileId)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func GetBootstrapMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method allowed!", http.StatusMethodNotAllowed)
		return
	}

	nodes, err := database.GetAllKeys(config.DB, "active_nodes")
	activeNodesCount := 0
	var nodeAddresses []string

	if err == nil {
		activeNodesCount = len(nodes)
		for _, key := range nodes {
			addr := strings.TrimPrefix(string(key), "active_nodes_")
			nodeAddresses = append(nodeAddresses, addr)
		}
	}

	manifests, err := database.GetAllKeys(config.DB, "manifests")
	totalFiles := 0
	if err == nil {
		totalFiles = len(manifests)
	}

	lsm, vlog := config.DB.Size()

	dbPath := os.Getenv("KAIROS_DB_PATH")
	if dbPath == "" {
		dbPath = "/data"
	}
	var stat syscall.Statfs_t
	var totalSpace, availableSpace, usedSpace uint64
	if err := syscall.Statfs(dbPath, &stat); err == nil {
		totalSpace = stat.Blocks * uint64(stat.Bsize)
		availableSpace = stat.Bavail * uint64(stat.Bsize)
		usedSpace = totalSpace - availableSpace
	}

	metrics := map[string]interface{}{
		"status": "online",
		"storage": map[string]interface{}{
			"database_used_bytes": lsm + vlog,
			"disk_total_bytes":    totalSpace,
			"disk_used_bytes":     usedSpace,
			"disk_free_bytes":     availableSpace,
		},
		"network": map[string]interface{}{
			"active_nodes":   activeNodesCount,
			"total_files":    totalFiles,
			"node_addresses": nodeAddresses,
		},
	}

	data, _ := json.Marshal(metrics)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
