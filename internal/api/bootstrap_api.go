package api

import (
	"crypto/sha256"
	"encoding/json"
	"log"
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
	log.Printf("[API: SubscribeNode] - [INFO] Incoming subscription request")
	var subscription models.SubscriptionRequest
	json.NewDecoder(r.Body).Decode(&subscription)
	defer r.Body.Close()

	message, _ := json.Marshal(models.SubscriptionRequest{Address: subscription.Address, PublicKey: subscription.PublicKey})
	check, _ := crypto.VerifySignature(message, subscription.Signature, subscription.PublicKey)

	if check {
		payload, _ := json.Marshal(models.ActiveNodeRecord{PublicKey: subscription.PublicKey, Timestamp: time.Now().UnixNano()})
		database.PutData(config.DB, "active_nodes", subscription.Address, payload)
		log.Printf("[API: SubscribeNode] - [SUCCESS] Node %s successfully subscribed to network", subscription.Address)
		w.WriteHeader(http.StatusOK)
	} else {
		log.Printf("[API: SubscribeNode] - [WARN] Unauthorized subscription attempt from address: %s", subscription.Address)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func SyncDigestHandler(w http.ResponseWriter, r *http.Request) {
	var digest models.SyncDigest
	json.NewDecoder(r.Body).Decode(&digest)
	defer r.Body.Close()

	message, _ := json.Marshal(models.SyncDigest{Address: digest.Address, PublicKey: digest.PublicKey, NodeKeys: digest.NodeKeys, ManifestKeys: digest.ManifestKeys})
	check, _ := crypto.VerifySignature(message, digest.Signature, digest.PublicKey)

	if !check {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	localNodeKeys, _ := database.GetAllKeys(config.DB, "active_nodes")
	localManifestKeys, _ := database.GetAllKeys(config.DB, "manifests")

	missingNodesInLocal, missingNodesInRemote := service.CompareKeys(localNodeKeys, digest.NodeKeys)
	missingManifestsInLocal, missingManifestsInRemote := service.CompareKeys(localManifestKeys, digest.ManifestKeys)

	payload := models.SyncPayload{
		Address:            config.AdvertisedAddress + ":" + strconv.Itoa(config.Port),
		PublicKey:          config.PublicKey,
		ActiveNodes:        make(map[string][]byte),
		FileManifests:      make(map[string][]byte),
		RequestedNodes:     missingNodesInLocal,
		RequestedManifests: missingManifestsInLocal,
	}

	for _, key := range missingNodesInRemote {
		if data, err := database.GetData(config.DB, "active_nodes", key); err == nil {
			payload.ActiveNodes[key] = data
		}
	}

	for _, key := range missingManifestsInRemote {
		if data, err := database.GetData(config.DB, "manifests", key); err == nil {
			payload.FileManifests[key] = data
		}
	}

	jsonBytes, _ := json.Marshal(payload)
	signature, _ := crypto.SignMessage(jsonBytes)
	payload.Signature = signature

	jsonBytes, _ = json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

func SyncPushHandler(w http.ResponseWriter, r *http.Request) {
	var payload models.SyncPayload
	json.NewDecoder(r.Body).Decode(&payload)
	defer r.Body.Close()

	message, _ := json.Marshal(models.SyncPayload{Address: payload.Address, PublicKey: payload.PublicKey, ActiveNodes: payload.ActiveNodes, FileManifests: payload.FileManifests, RequestedNodes: payload.RequestedNodes, RequestedManifests: payload.RequestedManifests})
	check, _ := crypto.VerifySignature(message, payload.Signature, payload.PublicKey)

	if check {
		go service.ProcessAlignment(payload.ActiveNodes, payload.FileManifests)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func RequestNodesForFileUploadAPI(w http.ResponseWriter, r *http.Request) {
	log.Printf("[API: RequestNodesForFileUpload] - [INFO] Node allocation requested")
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
			response = allDBNodes[:nodesToPickup]
		}

		log.Printf("[API: RequestNodesForFileUpload] - [SUCCESS] Allocated %d nodes for upload to %s", len(response), request.Address)
		jsonBytes, _ := json.Marshal(response)
		w.Write(jsonBytes)
	} else {
		log.Printf("[API: RequestNodesForFileUpload] - [WARN] Unauthorized node request from %s", request.Address)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func InsertFileManifest(w http.ResponseWriter, r *http.Request) {
	var request models.FileManifestRequest
	json.NewDecoder(r.Body).Decode(&request)
	defer r.Body.Close()

	log.Printf("[API: InsertFileManifest] - [INFO] Request to insert manifest for FileID: %s", request.Manifest.FileId)

	manifestBytes, _ := json.Marshal(request.Manifest)
	hashToVerify := sha256.Sum256(manifestBytes)

	check, _ := crypto.VerifySignature(hashToVerify[:], request.Signature, request.PublicKey)
	if check {
		database.PutData(config.DB, "manifests", request.Manifest.FileId, manifestBytes)
		log.Printf("[API: InsertFileManifest] - [SUCCESS] Manifest securely stored for FileID: %s", request.Manifest.FileId)
		w.WriteHeader(http.StatusOK)
	} else {
		log.Printf("[API: InsertFileManifest] - [WARN] Unauthorized manifest insertion attempt for FileID: %s", request.Manifest.FileId)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func DownloadFileManifest(w http.ResponseWriter, r *http.Request) {
	var request models.GetFileManifestRequest
	json.NewDecoder(r.Body).Decode(&request)
	defer r.Body.Close()

	log.Printf("[API: DownloadFileManifest] - [INFO] Request to download manifest for FileID: %s", request.FileId)

	message, _ := json.Marshal(models.GetFileManifestRequest{Address: request.Address, PublicKey: request.PublicKey, FileId: request.FileId})
	check, _ := crypto.VerifySignature(message, request.Signature, request.PublicKey)

	if check {
		dbData, _ := database.GetData(config.DB, "manifests", request.FileId)
		log.Printf("[API: DownloadFileManifest] - [SUCCESS] Manifest dispatched for FileID: %s", request.FileId)
		w.Write(dbData)
	} else {
		log.Printf("[API: DownloadFileManifest] - [WARN] Unauthorized manifest download attempt for FileID: %s", request.FileId)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func DeleteFileManifest(w http.ResponseWriter, r *http.Request) {
	var request models.GetFileManifestRequest
	json.NewDecoder(r.Body).Decode(&request)
	defer r.Body.Close()

	log.Printf("[API: DeleteFileManifest] - [INFO] Request to delete manifest for FileID: %s", request.FileId)

	message, _ := json.Marshal(models.GetFileManifestRequest{Address: request.Address, PublicKey: request.PublicKey, FileId: request.FileId})
	check, _ := crypto.VerifySignature(message, request.Signature, request.PublicKey)

	if check {
		database.DeleteKey(config.DB, "manifests", request.FileId)
		log.Printf("[API: DeleteFileManifest] - [SUCCESS] Manifest deleted for FileID: %s", request.FileId)
		w.WriteHeader(http.StatusOK)
	} else {
		log.Printf("[API: DeleteFileManifest] - [WARN] Unauthorized manifest deletion attempt for FileID: %s", request.FileId)
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
