package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strconv"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/crypto"
	"github.com/FraMan97/kairos-p2p-engine/internal/models"
)

func SubscribeNode() error {
	if len(config.BootStrapServers) == 0 {
		log.Printf("[Subscription] - [WARN] No bootstrap servers configured. Skipping subscription.")
		return nil
	}

	chosenServer := config.BootStrapServers[rand.Intn(len(config.BootStrapServers))]
	nodeAddress := config.AdvertisedAddress + ":" + strconv.Itoa(config.Port)

	subscription := models.SubscriptionRequest{
		Address:   nodeAddress,
		PublicKey: config.PublicKey,
	}

	jsonBytes, err := json.Marshal(subscription)
	if err != nil {
		log.Printf("[Subscription] - [ERROR] Failed to marshal subscription request: %v", err)
		return err
	}

	signature, err := crypto.SignMessage(jsonBytes)
	if err != nil {
		log.Printf("[Subscription] - [ERROR] Failed to sign subscription request: %v", err)
		return err
	}
	subscription.Signature = signature

	jsonBytes, _ = json.Marshal(subscription)

	targetURL := fmt.Sprintf("http://%s/subscribe", chosenServer)
	log.Printf("[Subscription] - [INFO] Attempting to subscribe to bootstrap node: %s", chosenServer)

	resp, err := config.HttpClient.Post(targetURL, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Printf("[Subscription] - [ERROR] Network failure while contacting bootstrap %s: %v", chosenServer, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("[Subscription] - [SUCCESS] Node %s successfully registered at %s", nodeAddress, chosenServer)
		return nil
	}

	log.Printf("[Subscription] - [WARN] Subscription rejected by %s. Status Code: %d", chosenServer, resp.StatusCode)
	return fmt.Errorf("subscription failed with status: %d", resp.StatusCode)
}