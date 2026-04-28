package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/models"
)

func CleanNodeDatabase(ctx context.Context) {
	log.Printf("[GC: NodeDatabase] - [INFO] Background worker started. Retention policy: 7 days after release")
	ticker := time.NewTicker(time.Duration(config.CronClean) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var keysToDelete []string

			database.IterateAndProcess(config.DB, "chunks", func(key string, val []byte) {
				var chunk models.ChunkRequest
				if err := json.NewDecoder(bytes.NewBuffer(val)).Decode(&chunk); err == nil {
					parsedTime, err := time.Parse(time.RFC3339, chunk.ReleaseDate)
					if err == nil && time.Now().UTC().After(parsedTime.Add(time.Hour*24*7)) {
						keysToDelete = append(keysToDelete, chunk.ChunkId)
					}
				}
			})

			removedCount := 0
			for _, key := range keysToDelete {
				if err := database.DeleteKey(config.DB, "chunks", key); err == nil {
					removedCount++
				}
			}

			if removedCount > 0 {
				log.Printf("[GC: NodeDatabase] - [SUCCESS] Cleanup finished. Removed %d expired chunks", removedCount)
			}

			database.RunValueLogGC(config.DB)

		case <-ctx.Done():
			log.Printf("[GC: NodeDatabase] - [INFO] Stopping background worker...")
			return
		}
	}
}

func CleanBootstrapDatabase(ctx context.Context) {
	log.Printf("[GC: BootstrapDatabase] - [INFO] Background worker started. Retention policy: 7 days after release")
	ticker := time.NewTicker(time.Duration(config.CronClean) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var keysToDelete []string

			database.IterateAndProcess(config.DB, "manifests", func(key string, val []byte) {
				var manifest models.FileManifest
				if err := json.NewDecoder(bytes.NewBuffer(val)).Decode(&manifest); err == nil {
					parsedTime, err := time.Parse(time.RFC3339, manifest.ReleaseDate)
					if err == nil && time.Now().UTC().After(parsedTime.Add(time.Hour*24*7)) {
						keysToDelete = append(keysToDelete, manifest.FileId)
					}
				}
			})

			removedCount := 0
			for _, key := range keysToDelete {
				if err := database.DeleteKey(config.DB, "manifests", key); err == nil {
					removedCount++
				}
			}

			if removedCount > 0 {
				log.Printf("[GC: BootstrapDatabase] - [SUCCESS] Cleanup finished. Removed %d expired manifests", removedCount)
			}

			database.RunValueLogGC(config.DB)

		case <-ctx.Done():
			log.Printf("[GC: BootstrapDatabase] - [INFO] Stopping background worker...")
			return
		}
	}
}

func CleanInactiveNodes(ctx context.Context) {
	log.Printf("[GC: InactiveNodes] - [INFO] Heartbeat monitoring started. Timeout: 2 minutes")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			allNodes, err := database.GetAllData(config.DB, "active_nodes")
			if err != nil {
				log.Printf("[GC: InactiveNodes] - [ERROR] Failed to retrieve nodes for monitoring: %v", err)
				continue
			}

			now := time.Now().UnixNano()
			removedCount := 0

			for addr, data := range allNodes {
				var record models.ActiveNodeRecord
				if err := json.NewDecoder(bytes.NewBuffer(data)).Decode(&record); err != nil {
					continue
				}

				if now-record.Timestamp > int64(2*time.Minute) {
					log.Printf("[GC: InactiveNodes] - [WARN] Node %s is unresponsive. Removing from registry...", addr)
					if err := database.DeleteKey(config.DB, "active_nodes", addr); err == nil {
						removedCount++
					}
				}
			}

			if removedCount > 0 {
				log.Printf("[GC: InactiveNodes] - [SUCCESS] Cleanup finished. Removed %d inactive nodes", removedCount)
			}

		case <-ctx.Done():
			log.Printf("[GC: InactiveNodes] - [INFO] Stopping monitoring worker...")
			return
		}
	}
}
