package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/FraMan97/kairos-p2p-engine/internal/api"
	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/crypto"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/service"
)

func main() {
	config.InitHttpClient()
	envPort := os.Getenv("PORT")
	if envPort != "" {
		config.Port, _ = strconv.Atoi(envPort)
	}

	config.AdvertisedAddress = os.Getenv("POD_IP")
	if config.AdvertisedAddress == "" {
		config.AdvertisedAddress = "localhost"
	}

	bootstrapServers := os.Getenv("BOOTSTRAP_SERVERS")
	if bootstrapServers != "" {
		config.BootStrapServers = strings.Split(bootstrapServers, ",")
	}

	dbPath := os.Getenv("KAIROS_DB_PATH")
	if dbPath == "" {
		dbPath = "./data-node"
	}

	config.PrivateKeyDir = dbPath + "/keys/private_key.pem"
	config.PublicKeyDir = dbPath + "/keys/public_key.pem"

	db, err := database.OpenDatabase(dbPath)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}

	if !crypto.KeysExist() {
		crypto.GenerateKeyPair()
	} else {
		config.PublicKey, _ = crypto.GetPublicKey()
		config.PrivateKey, _ = crypto.GetPrivateKey()
	}

	if len(config.BootStrapServers) > 0 {
		go func() {
			for {
				if err := service.SubscribeNode(); err == nil {
					time.Sleep(1 * time.Minute)
				} else {
					time.Sleep(5 * time.Second)
				}
			}
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	go service.CleanNodeDatabase(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/put", api.PutFile)
	mux.HandleFunc("/get", api.GetFile)
	mux.HandleFunc("/delete", api.DeleteFile)
	mux.HandleFunc("/chunk", api.Chunk)
	mux.HandleFunc("/upload/status", api.CheckStatus)
	mux.HandleFunc("/metrics", api.GetNodeMetrics)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("[Node] Listening on %d", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Node Gracefully...")
	cancel()
	server.Shutdown(context.Background())
	db.Close()
	log.Println("Node stopped")
}
