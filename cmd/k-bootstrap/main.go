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
		dbPath = "./data-bootstrap"
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

	ctx, cancel := context.WithCancel(context.Background())
	go service.ServerBootstrapSync(ctx)
	go service.CleanBootstrapDatabase(ctx)
	go service.CleanInactiveNodes(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", api.SubsribeNode)
	mux.HandleFunc("/synchronize", api.SynchronizeData)
	mux.HandleFunc("/file/nodes", api.RequestNodesForFileUploadAPI)
	mux.HandleFunc("/file/manifest", api.InsertFileManifest)
	mux.HandleFunc("/manifests", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			api.DownloadFileManifest(w, r)
		case http.MethodDelete:
			api.DeleteFileManifest(w, r)
		}
	})
	mux.HandleFunc("/metrics", api.GetBootstrapMetrics)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("[Bootstrap] Listening on %d", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Bootstrap Gracefully...")
	cancel()
	server.Shutdown(context.Background())
	db.Close()
	log.Println("Bootstrap stopped")
}
