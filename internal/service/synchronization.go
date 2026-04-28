package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/crypto"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/models"
)

func ServerBootstrapSync(ctx context.Context) {
	log.Printf("[Sync: Bootstrap] - [INFO] Background delta-sync worker started. Interval: %d seconds", config.CronSync)
	ticker := time.NewTicker(time.Duration(config.CronSync) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			performSync()
		case <-ctx.Done():
			log.Printf("[Sync: Bootstrap] - [INFO] Stopping background worker...")
			return
		}
	}
}

func performSync() {
	if len(config.BootStrapServers) <= 1 {
		return
	}

	chosenServer := config.BootStrapServers[rand.Intn(len(config.BootStrapServers))]
	selfAddress := config.AdvertisedAddress + ":" + strconv.Itoa(config.Port)

	if chosenServer == selfAddress {
		return
	}

	log.Printf("[Sync: Bootstrap] - [INFO] Initiating Delta-Sync with peer: %s", chosenServer)

	nodeKeys, _ := database.GetAllKeys(config.DB, "active_nodes")
	manifestKeys, _ := database.GetAllKeys(config.DB, "manifests")

	digest := models.SyncDigest{
		Address:      selfAddress,
		PublicKey:    config.PublicKey,
		NodeKeys:     nodeKeys,
		ManifestKeys: manifestKeys,
	}

	jsonBytes, err := json.Marshal(digest)
	if err != nil {
		return
	}

	signature, err := crypto.SignMessage(jsonBytes)
	if err != nil {
		return
	}
	digest.Signature = signature
	jsonBytes, _ = json.Marshal(digest)

	targetURL := fmt.Sprintf("http://%s/sync/digest", chosenServer)
	resp, err := config.HttpClient.Post(targetURL, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Printf("[Sync: Bootstrap] - [ERROR] Peer %s unreachable: %v", chosenServer, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var payload models.SyncPayload
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return
		}

		ProcessAlignment(payload.ActiveNodes, payload.FileManifests)

		if len(payload.RequestedNodes) > 0 || len(payload.RequestedManifests) > 0 {
			pushMissingData(chosenServer, payload.RequestedNodes, payload.RequestedManifests)
		}
	}
}

func pushMissingData(targetServer string, reqNodes []string, reqManifests []string) {
	selfAddress := config.AdvertisedAddress + ":" + strconv.Itoa(config.Port)
	nodesData := make(map[string][]byte)
	manifestsData := make(map[string][]byte)

	for _, key := range reqNodes {
		if data, err := database.GetData(config.DB, "active_nodes", key); err == nil {
			nodesData[key] = data
		}
	}

	for _, key := range reqManifests {
		if data, err := database.GetData(config.DB, "manifests", key); err == nil {
			manifestsData[key] = data
		}
	}

	payload := models.SyncPayload{
		Address:       selfAddress,
		PublicKey:     config.PublicKey,
		ActiveNodes:   nodesData,
		FileManifests: manifestsData,
	}

	jsonBytes, _ := json.Marshal(payload)
	signature, _ := crypto.SignMessage(jsonBytes)
	payload.Signature = signature
	jsonBytes, _ = json.Marshal(payload)

	targetURL := fmt.Sprintf("http://%s/sync/push", targetServer)
	resp, err := config.HttpClient.Post(targetURL, "application/json", bytes.NewBuffer(jsonBytes))
	if err == nil {
		resp.Body.Close()
	}
}

func ProcessAlignment(nodes map[string][]byte, manifests map[string][]byte) {
	nodesUpdated := 0
	manifestsUpdated := 0

	for k, v := range nodes {
		if err := database.PutData(config.DB, "active_nodes", k, v); err == nil {
			nodesUpdated++
		}
	}

	for k, v := range manifests {
		check, err := database.ExistsKey(config.DB, "manifests", k)
		if err == nil && !check {
			if err := database.PutData(config.DB, "manifests", k, v); err == nil {
				manifestsUpdated++
			}
		}
	}

	if nodesUpdated > 0 || manifestsUpdated > 0 {
		log.Printf("[Sync: Alignment] - [SUCCESS] Database aligned: %d nodes updated, %d new manifests stored", nodesUpdated, manifestsUpdated)
	}
}

func CompareKeys(localKeys, remoteKeys []string) (missingInLocal, missingInRemote []string) {
	localMap := make(map[string]bool)
	for _, k := range localKeys {
		localMap[k] = true
	}

	remoteMap := make(map[string]bool)
	for _, k := range remoteKeys {
		remoteMap[k] = true
		if !localMap[k] {
			missingInLocal = append(missingInLocal, k)
		}
	}

	for _, k := range localKeys {
		if !remoteMap[k] {
			missingInRemote = append(missingInRemote, k)
		}
	}

	return missingInLocal, missingInRemote
}
