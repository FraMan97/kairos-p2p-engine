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

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/service"
	"github.com/google/uuid"
)

type UploadStatus struct {
	Status string `json:"status"`
}

func saveStatus(id string, status string) {
	s := UploadStatus{Status: status}
	bytes, _ := json.Marshal(s)
	err := database.PutData(config.DB, "upload_status", id, bytes)
	if err != nil {
		log.Printf("[UploadStatus] Error saving status for %s: %v", id, err)
	}
}

func PutFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method allowed!", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(32 << 20)

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	releaseTime := r.FormValue("release_time")
	tempDir := os.TempDir()
	tempFilePath := filepath.Join(tempDir, "kairos_upload_"+uuid.New().String())

	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		http.Error(w, "Server Error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		os.Remove(tempFilePath)
		http.Error(w, "Upload Error", http.StatusInternalServerError)
		return
	}
	tempFile.Close()

	futureFileID := uuid.New().String()
	saveStatus(futureFileID, "processing")

	go func(fPath string, fHeader *multipart.FileHeader, rTime string, fId string) {
		defer func() {
			os.Remove(fPath)
			if r := recover(); r != nil {
				saveStatus(fId, "error")
			}
		}()

		localFile, err := os.Open(fPath)
		if err != nil {
			saveStatus(fId, "error")
			return
		}
		defer localFile.Close()

		blockSize := config.TargetChunkSize * config.DataShards

		err = service.StreamAndUploadFile(localFile, fHeader, blockSize, rTime, fId)
		if err != nil {
			log.Printf("[AsyncWorker] CRITICAL error %s: %v", fId, err)
			saveStatus(fId, "error")
			return
		}

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
		http.Error(w, "Missing id parameter", http.StatusBadRequest)
		return
	}

	data, err := database.GetData(config.DB, "upload_status", id)
	if err != nil {
		http.Error(w, "Status not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func GetFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET method allowed!", http.StatusMethodNotAllowed)
		return
	}

	fileId := r.URL.Query().Get("id")
	if fileId == "" {
		http.Error(w, "Missing fileId", http.StatusBadRequest)
		return
	}

	fileManifest, err := service.GetFileManifestFromServer(fileId)
	if err != nil {
		http.Error(w, "Error retrieving manifest", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileManifest.FileName))
	w.Header().Set("X-File-ID", fileId)

	hasher := sha256.New()
	multiWriter := io.MultiWriter(w, hasher)

	err = service.StreamReconstruct(multiWriter, fileManifest)
	if err != nil {
		log.Printf("[GetFile] - Streaming error: %v", err)
		return
	}

	finalHash := hex.EncodeToString(hasher.Sum(nil))
	if finalHash != fileManifest.HashFile {
		log.Printf("[GetFile] Hash mismatch! Expected %s, got %s", fileManifest.HashFile, finalHash)
	}
}

func DeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Only DELETE allowed", http.StatusMethodNotAllowed)
		return
	}

	fileId := r.URL.Query().Get("id")
	manifest, err := service.GetFileManifestFromServer(fileId)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	for _, block := range manifest.Split {
		for _, chunk := range block.Chunks {
			for _, nodeAddr := range chunk.Nodes {
				go service.RequestChunkDeletion(nodeAddr, chunk.ChunkId)
			}
		}
	}

	service.DeleteManifestOnBootstrap(fileId)
	w.WriteHeader(http.StatusOK)
}

func Chunk(w http.ResponseWriter, r *http.Request) {
	chunkId := r.URL.Query().Get("id")
	if r.Method == http.MethodPost {
		err := service.SaveChunk(r)
		if err != nil {
			http.Error(w, "Error saving chunk", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	} else if r.Method == http.MethodGet {
		chunk, err := service.GetChunk(r)
		if err != nil {
			http.Error(w, "Error getting chunk", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(chunk)
	} else if r.Method == http.MethodDelete {
		database.DeleteKey(config.DB, "chunks", chunkId)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
