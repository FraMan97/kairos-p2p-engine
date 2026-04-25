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
			allChunks, err := database.GetAllData(config.DB, "chunks")
			if err != nil {
				log.Printf("[GC: NodeDatabase] - [ERROR] Failed to retrieve chunks for cleaning: %v", err)
				continue
			}

			if len(allChunks) == 0 {
				continue
			}

			removedCount := 0
			for _, m := range allChunks {
				var chunk models.ChunkRequest
				if err := json.NewDecoder(bytes.NewBuffer(m)).Decode(&chunk); err != nil {
					continue
				}

				parsedTime, err := time.Parse(time.RFC3339, chunk.ReleaseDate)
				if err == nil && time.Now().UTC().After(parsedTime.Add(time.Hour*24*7)) {
					if err := database.DeleteKey(config.DB, "chunks", chunk.ChunkId); err == nil {
						removedCount++
					}
				}
			}

			if removedCount > 0 {
				log.Printf("[GC: NodeDatabase] - [SUCCESS] Cleanup finished. Removed %d expired chunks", removedCount)
			}

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
			allManifests, err := database.GetAllData(config.DB, "manifests")
			if err != nil {
				log.Printf("[GC: BootstrapDatabase] - [ERROR] Failed to retrieve manifests for cleaning: %v", err)
				continue
			}

			if len(allManifests) == 0 {
				continue
			}

			removedCount := 0
			for _, m := range allManifests {
				var manifest models.FileManifest
				if err := json.NewDecoder(bytes.NewBuffer(m)).Decode(&manifest); err != nil {
					continue
				}

				parsedTime, err := time.Parse(time.RFC3339, manifest.ReleaseDate)
				if err == nil && time.Now().UTC().After(parsedTime.Add(time.Hour*24*7)) {
					if err := database.DeleteKey(config.DB, "manifests", manifest.FileId); err == nil {
						removedCount++
					}
				}
			}

			if removedCount > 0 {
				log.Printf("[GC: BootstrapDatabase] - [SUCCESS] Cleanup finished. Removed %d expired manifests", removedCount)
			}

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