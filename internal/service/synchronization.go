package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/crypto"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/models"
)

func ServerBootstrapSync(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(config.CronSync) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			performSync()
		case <-ctx.Done():
			return
		}
	}
}

func performSync() {
	if len(config.BootStrapServers) <= 1 {
		return
	}
	chosenServer := rand.Intn(len(config.BootStrapServers))
	if config.BootStrapServers[chosenServer] == config.AdvertisedAddress+":"+strconv.Itoa(config.Port) {
		return
	}

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

	var receivedData models.SynchronizationRequest
	resp, err := config.HttpClient.Post(fmt.Sprintf("http://%s/synchronize", config.BootStrapServers[chosenServer]),
		"application/json", bytes.NewBuffer(jsonBytes))

	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		json.NewDecoder(resp.Body).Decode(&receivedData)
		ProcessAlignment(dataToExchange, receivedData)
	}
}

func ProcessAlignment(dbData models.SynchronizationRequest, receivedData models.SynchronizationRequest) {
	for k, v := range receivedData.ActiveNodes {
		database.PutData(config.DB, "active_nodes", k, v)
	}
	for k, v := range receivedData.FileManifests {
		check, _ := database.ExistsKey(config.DB, "manifests", k)
		if !check {
			database.PutData(config.DB, "manifests", k, v)
		}
	}
}
