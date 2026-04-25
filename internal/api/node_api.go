package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"syscall"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/service"
	"github.com/google/uuid"
)

type UploadStatus struct {
	Status string `json:"status"`
}

func saveStatus(id string, status string) {
	log.Printf("[UploadStatus] - [INFO] Updating status to '%s' for FileID: %s", status, id)
	s := UploadStatus{Status: status}
	bytes, _ := json.Marshal(s)
	err := database.PutData(config.DB, "upload_status", id, bytes)
	if err != nil {
		log.Printf("[UploadStatus] - [ERROR] Error saving status for %s: %v", id, err)
	}
}

func PutFile(w http.ResponseWriter, r *http.Request) {
	log.Printf("[API: PutFile] - [INFO] Incoming file upload request")
	if r.Method != http.MethodPost {
		log.Printf("[API: PutFile] - [WARN] Method not allowed: %s", r.Method)
		http.Error(w, "Only POST method allowed!", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(32 << 20)

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("[API: PutFile] - [ERROR] Failed to retrieve file from form data: %v", err)
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	releaseTime := r.FormValue("release_time")
	log.Printf("[API: PutFile] - [INFO] Received file '%s' (Size: %d). Release time set to: %s", header.Filename, header.Size, releaseTime)

	tempDir := os.TempDir()
	tempFilePath := filepath.Join(tempDir, "kairos_upload_"+uuid.New().String())

	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		log.Printf("[API: PutFile] - [ERROR] Failed to create temp file: %v", err)
		http.Error(w, "Server Error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		os.Remove(tempFilePath)
		log.Printf("[API: PutFile] - [ERROR] Failed to copy data to temp file: %v", err)
		http.Error(w, "Upload Error", http.StatusInternalServerError)
		return
	}
	tempFile.Close()

	futureFileID := uuid.New().String()
	log.Printf("[API: PutFile] - [SUCCESS] Temp file saved. Triggering async worker for FileID: %s", futureFileID)
	saveStatus(futureFileID, "processing")

	go func(fPath string, fHeader *multipart.FileHeader, rTime string, fId string) {
		defer func() {
			os.Remove(fPath)
			log.Printf("[AsyncWorker] - [INFO] Cleaned up temporary file: %s", fPath)
			if r := recover(); r != nil {
				log.Printf("[AsyncWorker] - [ERROR] Panic recovered during processing FileID %s: %v", fId, r)
				saveStatus(fId, "error")
			}
		}()

		localFile, err := os.Open(fPath)
		if err != nil {
			log.Printf("[AsyncWorker] - [ERROR] Failed to open temp file %s: %v", fPath, err)
			saveStatus(fId, "error")
			return
		}
		defer localFile.Close()

		blockSize := config.TargetChunkSize * config.DataShards

		err = service.StreamAndUploadFile(localFile, fHeader, blockSize, rTime, fId)
		if err != nil {
			log.Printf("[AsyncWorker] - [ERROR] CRITICAL failure for FileID %s: %v", fId, err)
			saveStatus(fId, "error")
			return
		}

		log.Printf("[AsyncWorker] - [SUCCESS] File processing and upload completed for FileID: %s", fId)
		saveStatus(fId, "completed")
	}(tempFilePath, header, releaseTime, futureFileID)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, futureFileID)
}

func CheckStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method allowed!", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		log.Printf("[API: CheckStatus] - [WARN] Missing id parameter")
		http.Error(w, "Missing id parameter", http.StatusBadRequest)
		return
	}

	data, err := database.GetData(config.DB, "upload_status", id)
	if err != nil {
		log.Printf("[API: CheckStatus] - [WARN] Status not found for FileID: %s", id)
		http.Error(w, "Status not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func GetFile(w http.ResponseWriter, r *http.Request) {
	log.Printf("[API: GetFile] - [INFO] Incoming file download request")
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method allowed!", http.StatusMethodNotAllowed)
		return
	}

	fileId := r.URL.Query().Get("id")
	if fileId == "" {
		log.Printf("[API: GetFile] - [WARN] Missing id parameter")
		http.Error(w, "Missing fileId", http.StatusBadRequest)
		return
	}

	log.Printf("[API: GetFile] - [INFO] Requesting manifest for FileID: %s", fileId)
	fileManifest, err := service.GetFileManifestFromServer(fileId)
	if err != nil {
		log.Printf("[API: GetFile] - [ERROR] Error retrieving manifest for FileID %s: %v", fileId, err)
		http.Error(w, "Error retrieving manifest", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileManifest.FileName))
	w.Header().Set("X-File-ID", fileId)

	hasher := sha256.New()
	multiWriter := io.MultiWriter(w, hasher)

	log.Printf("[API: GetFile] - [INFO] Starting stream reconstruction for FileID: %s", fileId)
	err = service.StreamReconstruct(multiWriter, fileManifest)
	if err != nil {
		log.Printf("[API: GetFile] - [ERROR] Streaming reconstruction failed for FileID %s: %v", fileId, err)
		return
	}

	finalHash := hex.EncodeToString(hasher.Sum(nil))
	if finalHash != fileManifest.HashFile {
		log.Printf("[API: GetFile] - [WARN] Hash mismatch for FileID %s! Expected: %s, Got: %s", fileId, fileManifest.HashFile, finalHash)
	} else {
		log.Printf("[API: GetFile] - [SUCCESS] File streamed and hash validated successfully for FileID: %s", fileId)
	}
}

func DeleteFile(w http.ResponseWriter, r *http.Request) {
	log.Printf("[API: DeleteFile] - [INFO] Incoming file deletion request")
	if r.Method != http.MethodDelete {
		http.Error(w, "Only DELETE allowed", http.StatusMethodNotAllowed)
		return
	}

	fileId := r.URL.Query().Get("id")
	if fileId == "" {
		log.Printf("[API: DeleteFile] - [WARN] Missing id parameter")
		http.Error(w, "Missing fileId", http.StatusBadRequest)
		return
	}

	log.Printf("[API: DeleteFile] - [INFO] Fetching manifest for deletion. FileID: %s", fileId)
	manifest, err := service.GetFileManifestFromServer(fileId)
	if err != nil {
		log.Printf("[API: DeleteFile] - [WARN] Manifest not found or error retrieving it for FileID: %s", fileId)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if manifest != nil && manifest.Split != nil {
		log.Printf("[API: DeleteFile] - [INFO] Dispatching chunk deletion requests for FileID: %s", fileId)
		for _, block := range manifest.Split {
			for _, chunk := range block.Chunks {
				for _, nodeAddr := range chunk.Nodes {
					go service.RequestChunkDeletion(nodeAddr, chunk.ChunkId)
				}
			}
		}
	} else {
		log.Printf("[API: DeleteFile] - [WARN] Manifest retrieved but contains no blocks for FileID: %s", fileId)
	}

	err = service.DeleteManifestOnBootstrap(fileId)
	if err != nil {
		log.Printf("[API: DeleteFile] - [ERROR] Failed to delete manifest on bootstrap: %v", err)
		http.Error(w, "Error deleting manifest", http.StatusInternalServerError)
		return
	}

	log.Printf("[API: DeleteFile] - [SUCCESS] File deletion process initiated for FileID: %s", fileId)
	w.WriteHeader(http.StatusOK)
}

func Chunk(w http.ResponseWriter, r *http.Request) {
	chunkId := r.URL.Query().Get("id")

	if r.Method == http.MethodPost {
		log.Printf("[API: Chunk] - [INFO] Received chunk POST request")
		err := service.SaveChunk(r)
		if err != nil {
			log.Printf("[API: Chunk] - [ERROR] Failed to save chunk: %v", err)
			http.Error(w, "Error saving chunk", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	} else if r.Method == http.MethodGet {
		chunk, err := service.GetChunk(r)
		if err != nil {
			log.Printf("[API: Chunk] - [ERROR] Failed to retrieve chunk: %v", err)
			http.Error(w, "Error getting chunk", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(chunk)

	} else if r.Method == http.MethodDelete {
		log.Printf("[API: Chunk] - [INFO] Received chunk DELETE request for ID: %s", chunkId)
		database.DeleteKey(config.DB, "chunks", chunkId)
		w.WriteHeader(http.StatusOK)

	} else {
		log.Printf("[API: Chunk] - [WARN] Invalid method %s on chunk endpoint", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func GetNodeMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method allowed!", http.StatusMethodNotAllowed)
		return
	}

	chunks, err := database.GetAllKeys(config.DB, "chunks")
	totalChunks := 0
	if err == nil {
		totalChunks = len(chunks)
	}

	lsm, vlog := config.DB.Size()
	badgerSize := lsm + vlog

	dbPath := os.Getenv("KAIROS_DB_PATH")
	if dbPath == "" {
		dbPath = "/data"
	}

	var stat syscall.Statfs_t
	var totalSpace, availableSpace, usedSpace uint64

	err = syscall.Statfs(dbPath, &stat)
	if err == nil {
		totalSpace = stat.Blocks * uint64(stat.Bsize)
		availableSpace = stat.Bavail * uint64(stat.Bsize)
		usedSpace = totalSpace - availableSpace
	}

	metrics := map[string]interface{}{
		"status": "online",
		"storage": map[string]interface{}{
			"database_used_bytes": badgerSize,
			"disk_total_bytes":    totalSpace,
			"disk_used_bytes":     usedSpace,
			"disk_free_bytes":     availableSpace,
		},
		"data": map[string]interface{}{
			"total_chunks": totalChunks,
		},
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		log.Printf("[API: GetNodeMetrics] - [ERROR] Failed to encode metrics: %v", err)
		http.Error(w, "Error encoding metrics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}