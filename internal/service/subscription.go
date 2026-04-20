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
		return nil
	}
	chosenServer := rand.Intn(len(config.BootStrapServers))
	subscription := models.SubscriptionRequest{
		Address:   config.AdvertisedAddress + ":" + strconv.Itoa(config.Port),
		PublicKey: config.PublicKey,
	}

	jsonBytes, _ := json.Marshal(subscription)
	signature, _ := crypto.SignMessage(jsonBytes)
	subscription.Signature = signature

	jsonBytes, _ = json.Marshal(subscription)
	resp, err := config.HttpClient.Post(fmt.Sprintf("http://%s/subscribe", config.BootStrapServers[chosenServer]),
		"application/json", bytes.NewBuffer(jsonBytes))

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("[Subscription] Subscribed to %s", config.BootStrapServers[chosenServer])
		return nil
	}
	return fmt.Errorf("subscription failed")
}
