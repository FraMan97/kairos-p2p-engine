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
	log.Printf("[Sync: Bootstrap] - [INFO] Background synchronization worker started. Interval: %d seconds", config.CronSync)
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

	log.Printf("[Sync: Bootstrap] - [INFO] Initiating data exchange with peer: %s", chosenServer)

	activeNodes, err := database.GetAllData(config.DB, "active_nodes")
	if err != nil {
		log.Printf("[Sync: Bootstrap] - [ERROR] Failed to fetch active nodes from DB: %v", err)
	}

	fileManifests, err := database.GetAllData(config.DB, "manifests")
	if err != nil {
		log.Printf("[Sync: Bootstrap] - [ERROR] Failed to fetch manifests from DB: %v", err)
	}

	dataToExchange := models.SynchronizationRequest{
		Address:       selfAddress,
		PublicKey:     config.PublicKey,
		ActiveNodes:   activeNodes,
		FileManifests: fileManifests,
	}

	jsonBytes, err := json.Marshal(dataToExchange)
	if err != nil {
		log.Printf("[Sync: Bootstrap] - [ERROR] Failed to marshal sync data: %v", err)
		return
	}

	signature, err := crypto.SignMessage(jsonBytes)
	if err != nil {
		log.Printf("[Sync: Bootstrap] - [ERROR] Failed to sign sync payload: %v", err)
		return
	}
	dataToExchange.Signature = signature

	jsonBytes, _ = json.Marshal(dataToExchange)

	targetURL := fmt.Sprintf("http://%s/synchronize", chosenServer)
	resp, err := config.HttpClient.Post(targetURL, "application/json", bytes.NewBuffer(jsonBytes))

	if err != nil {
		log.Printf("[Sync: Bootstrap] - [ERROR] Peer %s unreachable: %v", chosenServer, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var receivedData models.SynchronizationRequest
		if err := json.NewDecoder(resp.Body).Decode(&receivedData); err != nil {
			log.Printf("[Sync: Bootstrap] - [ERROR] Failed to decode response from %s: %v", chosenServer, err)
			return
		}

		log.Printf("[Sync: Bootstrap] - [SUCCESS] Payload received from %s. Nodes: %d, Manifests: %d",
			chosenServer, len(receivedData.ActiveNodes), len(receivedData.FileManifests))

		ProcessAlignment(dataToExchange, receivedData)
	} else {
		log.Printf("[Sync: Bootstrap] - [WARN] Peer %s returned unexpected status: %d", chosenServer, resp.StatusCode)
	}
}

func ProcessAlignment(dbData models.SynchronizationRequest, receivedData models.SynchronizationRequest) {
	nodesUpdated := 0
	manifestsUpdated := 0

	for k, v := range receivedData.ActiveNodes {
		if err := database.PutData(config.DB, "active_nodes", k, v); err == nil {
			nodesUpdated++
		}
	}

	for k, v := range receivedData.FileManifests {
		check, err := database.ExistsKey(config.DB, "manifests", k)
		if err == nil && !check {
			if err := database.PutData(config.DB, "manifests", k, v); err == nil {
				manifestsUpdated++
			}
		}
	}

	if nodesUpdated > 0 || manifestsUpdated > 0 {
		log.Printf("[Sync: Alignment] - [SUCCESS] Database aligned: %d nodes updated, %d new manifests stored",
			nodesUpdated, manifestsUpdated)
	}
}