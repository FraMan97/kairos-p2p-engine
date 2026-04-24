package service

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/models"
)

func CleanNodeDatabase(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(config.CronClean) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			allChunks, _ := database.GetAllData(config.DB, "chunks")
			for _, m := range allChunks {
				var chunk models.ChunkRequest
				json.NewDecoder(bytes.NewBuffer(m)).Decode(&chunk)
				parsedTime, err := time.Parse(time.RFC3339, chunk.ReleaseDate)
				if err == nil && time.Now().UTC().After(parsedTime.Add(time.Hour*24*7)) {
					database.DeleteKey(config.DB, "chunks", chunk.ChunkId)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func CleanBootstrapDatabase(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(config.CronClean) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			allManifests, _ := database.GetAllData(config.DB, "manifests")
			for _, m := range allManifests {
				var manifest models.FileManifest
				json.NewDecoder(bytes.NewBuffer(m)).Decode(&manifest)
				parsedTime, err := time.Parse(time.RFC3339, manifest.ReleaseDate)
				if err == nil && time.Now().UTC().After(parsedTime.Add(time.Hour*24*7)) {
					database.DeleteKey(config.DB, "manifests", manifest.FileId)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func CleanInactiveNodes(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			allNodes, _ := database.GetAllData(config.DB, "active_nodes")
			now := time.Now().UnixNano()
			for addr, data := range allNodes {
				var record models.ActiveNodeRecord
				json.NewDecoder(bytes.NewBuffer(data)).Decode(&record)
				if now-record.Timestamp > int64(2*time.Minute) {
					database.DeleteKey(config.DB, "active_nodes", addr)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
