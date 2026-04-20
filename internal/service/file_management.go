package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/crypto"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/models"
	"github.com/corvus-ch/shamir"
	"github.com/drand/tlock"
	tlock_http "github.com/drand/tlock/networks/http"
	"github.com/google/uuid"
	"github.com/klauspost/reedsolomon"
)

func StreamAndUploadFile(file *os.File, header *multipart.FileHeader, blockSize int, releaseTime string, fileId string) error {
	log.Printf("[StreamUpload] - Starting streaming process for FileID: %s", fileId)

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("error hashing file: %v", err)
	}
	fileHash := hex.EncodeToString(hasher.Sum(nil))

	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("error seeking file: %v", err)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("error getting file stats: %v", err)
	}
	fileSize := fileInfo.Size()

	totalBlocks := int((fileSize + int64(blockSize) - 1) / int64(blockSize))
	if totalBlocks == 0 {
		totalBlocks = 1
	}

	nodes, err := RequestNodesForFileUpload(totalBlocks * config.TotalShards)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %v", err)
	}

	drandRound, err := GetRoundForTime(releaseTime)
	if err != nil {
		return fmt.Errorf("error getting drand round: %v", err)
	}

	tNetwork, err := tlock_http.NewNetwork(config.DrandRelays[rand.Intn(len(config.DrandRelays))], config.DrandChainHash)
	if err != nil {
		return fmt.Errorf("drand network error: %v", err)
	}
	tlockClient := tlock.New(tNetwork)

	enc, err := reedsolomon.New(config.DataShards, config.ParityShards)
	if err != nil {
		return fmt.Errorf("rs error: %v", err)
	}

	fileManifest := &models.FileManifest{
		FileName:          header.Filename,
		FileId:            fileId,
		FileSize:          fileSize,
		ReleaseDate:       releaseTime,
		HashFile:          fileHash,
		HashAlgorithm:     "SHA256",
		Blocks:            totalBlocks,
		ChunksPerBlocks:   config.TotalShards,
		ReedSolomonConfig: models.ReedSolomonConfig{DataShards: config.DataShards, ParityShards: config.ParityShards},
		Split:             make(map[int]models.FileBlock),
	}

	type uploadJob struct {
		chunkReq    models.ChunkRequest
		targetNodes []string
	}
	jobs := make(chan uploadJob, config.TotalShards*2)
	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	workerCount := 5
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				jsonBytes, err := json.Marshal(job.chunkReq)
				if err != nil {
					continue
				}
				signature, err := crypto.SignMessage(jsonBytes)
				if err != nil {
					continue
				}
				job.chunkReq.Signature = signature
				jsonBytes, _ = json.Marshal(job.chunkReq)

				success := false
				for _, nodeAddr := range job.targetNodes {
					resp, err := config.HttpClient.Post(fmt.Sprintf("http://%s/chunk", nodeAddr), "application/json", bytes.NewBuffer(jsonBytes))
					if err == nil {
						defer resp.Body.Close()
						if resp.StatusCode == 200 {
							success = true
							break
						}
					}
				}
				if !success {
					select {
					case errChan <- fmt.Errorf("failed to upload chunk %s", job.chunkReq.ChunkId):
					default:
					}
				}
			}
		}()
	}

	buffer := make([]byte, blockSize)
	blockID := 0

	for {
		n, err := io.ReadFull(file, buffer)
		if err == io.EOF {
			break
		}
		if err == io.ErrUnexpectedEOF {
			buffer = buffer[:n]
			err = nil
		} else if err != nil {
			close(jobs)
			return err
		}

		key := crypto.GenerateRandomAESKey()
		var encryptedKeyBuf bytes.Buffer
		err = tlockClient.Encrypt(&encryptedKeyBuf, bytes.NewReader(key), drandRound)
		if err != nil {
			close(jobs)
			return err
		}
		encryptedKey := encryptedKeyBuf.Bytes()

		encryptedBlock, err := crypto.EncryptGCM(buffer, key)
		if err != nil {
			close(jobs)
			return err
		}

		dataChunks, err := enc.Split(encryptedBlock)
		if err != nil {
			close(jobs)
			return err
		}
		err = enc.Encode(dataChunks)
		if err != nil {
			close(jobs)
			return err
		}

		keyParts, err := shamir.Split(encryptedKey, config.TotalShards, config.DataShards)
		if err != nil {
			close(jobs)
			return err
		}

		var shamirIndexes []byte
		for k := range keyParts {
			shamirIndexes = append(shamirIndexes, k)
		}

		fileBlock := models.FileBlock{
			EncryptedBlockSize: len(encryptedBlock),
			Chunks:             make([]models.Chunk, 0, config.TotalShards),
		}

		for i := 0; i < config.TotalShards; i++ {
			payloadDati := dataChunks[i]
			currentIndex := shamirIndexes[i]
			rawKeyPart := keyParts[currentIndex]

			selectedNodes := pickRandomItems(nodes, config.ChunksTolerance)
			chunkId := uuid.New().String()

			chunkMeta := models.Chunk{
				ShardIndex:   i,
				KeyIndexPart: currentIndex,
				KeyPart:      rawKeyPart,
				Nodes:        selectedNodes,
				ChunkId:      chunkId,
			}
			fileBlock.Chunks = append(fileBlock.Chunks, chunkMeta)

			dataSafe := make([]byte, len(payloadDati))
			copy(dataSafe, payloadDati)

			jobs <- uploadJob{
				chunkReq: models.ChunkRequest{
					Address:     config.AdvertisedAddress + ":" + strconv.Itoa(config.Port),
					PublicKey:   config.PublicKey,
					ChunkId:     chunkId,
					Shard:       dataSafe,
					ReleaseDate: releaseTime,
				},
				targetNodes: selectedNodes,
			}
		}

		fileManifest.Split[blockID] = fileBlock
		blockID++

		if n < blockSize {
			break
		}
	}

	close(jobs)
	wg.Wait()

	select {
	case err := <-errChan:
		return fmt.Errorf("upload pipeline failed: %v", err)
	default:
	}

	err = UploadFileManifest(fileManifest)
	if err != nil {
		return fmt.Errorf("manifest upload failed: %v", err)
	}

	return nil
}

func StreamReconstruct(w io.Writer, fileManifest *models.FileManifest) error {
	tNetwork, err := tlock_http.NewNetwork(config.DrandRelays[rand.Intn(len(config.DrandRelays))], config.DrandChainHash)
	if err != nil {
		return fmt.Errorf("network tlock error: %v", err)
	}
	tlockClient := tlock.New(tNetwork)

	enc, err := reedsolomon.New(fileManifest.ReedSolomonConfig.DataShards, fileManifest.ReedSolomonConfig.ParityShards)
	if err != nil {
		return fmt.Errorf("Reed-Solomon creation error: %v", err)
	}

	shardsRequired := fileManifest.ReedSolomonConfig.DataShards
	totalShards := fileManifest.ReedSolomonConfig.DataShards + fileManifest.ReedSolomonConfig.ParityShards

	for i := 0; i < fileManifest.Blocks; i++ {
		blockData := fileManifest.Split[i]

		shards := make([][]byte, totalShards)
		keyParts := make(map[byte][]byte)
		shardsReceived := 0

		originalChunkInfo := make(map[string]models.Chunk)
		for _, c := range blockData.Chunks {
			originalChunkInfo[c.ChunkId] = c
		}

		for _, chunkMeta := range blockData.Chunks {
			if shardsReceived >= shardsRequired {
				break
			}

			for _, nodeAddr := range chunkMeta.Nodes {
				chunkReq, err := RequestChunk(nodeAddr, chunkMeta.ChunkId)
				if err != nil {
					continue
				}

				originalInfo := originalChunkInfo[chunkReq.ChunkId]

				if shards[originalInfo.ShardIndex] != nil {
					continue
				}

				shards[originalInfo.ShardIndex] = chunkReq.Shard
				keyParts[originalInfo.KeyIndexPart] = originalInfo.KeyPart
				shardsReceived++
				break
			}
		}

		if shardsReceived < shardsRequired {
			return fmt.Errorf("insufficient data for block %d", i)
		}

		err = enc.Reconstruct(shards)
		if err != nil {
			return fmt.Errorf("reconstruction failed block %d: %v", i, err)
		}

		var encryptedBlock bytes.Buffer
		err = enc.Join(&encryptedBlock, shards, blockData.EncryptedBlockSize)
		if err != nil {
			return fmt.Errorf("failed to join shards: %v", err)
		}

		encryptedAESKey, err := shamir.Combine(keyParts)
		if err != nil {
			return fmt.Errorf("failed to combine Shamir key: %v", err)
		}

		var plainAESKeyBuf bytes.Buffer
		err = tlockClient.Decrypt(&plainAESKeyBuf, bytes.NewReader(encryptedAESKey))
		if err != nil {
			return fmt.Errorf("failed to unlock key with drand: %v", err)
		}

		decryptedBlock, err := crypto.DecryptGCM(encryptedBlock.Bytes(), plainAESKeyBuf.Bytes())
		if err != nil {
			return fmt.Errorf("failed to decrypt block AES: %v", err)
		}

		if _, err := w.Write(decryptedBlock); err != nil {
			return fmt.Errorf("client connection lost: %v", err)
		}
	}

	return nil
}

func GetRoundForTime(releaseTimeStr string) (uint64, error) {
	targetTime, err := time.Parse(time.RFC3339, releaseTimeStr)
	if err != nil {
		return 0, err
	}
	net, err := tlock_http.NewNetwork(config.DrandRelays[rand.Intn(len(config.DrandRelays))], config.DrandChainHash)
	if err != nil {
		return 0, err
	}
	round := net.Current(targetTime)
	if round == 0 {
		return 0, err
	}
	return round, nil
}

func RequestNodesForFileUpload(totalChunks int) ([]string, error) {
	chosenServer := rand.Intn(len(config.BootStrapServers))
	request := models.NodesForFileUploadRequest{
		Address:       config.AdvertisedAddress + ":" + strconv.Itoa(config.Port),
		PublicKey:     config.PublicKey,
		TotalChunks:   totalChunks,
		NodesPerChunk: config.ChunksTolerance,
	}
	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	signature, err := crypto.SignMessage(jsonBytes)
	if err != nil {
		return nil, err
	}
	request.Signature = signature
	jsonBytes, err = json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := config.HttpClient.Post(fmt.Sprintf("http://%s/file/nodes", config.BootStrapServers[chosenServer]), "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var response []string
		json.NewDecoder(resp.Body).Decode(&response)
		return response, nil
	}
	return nil, fmt.Errorf("error with message: %v", resp.Body)
}

func UploadFileManifest(fileManifest *models.FileManifest) error {
	chosenServer := rand.Intn(len(config.BootStrapServers))
	manifestBytes, err := json.Marshal(*fileManifest)
	if err != nil {
		return err
	}
	hashToSign := sha256.Sum256(manifestBytes)
	signature, err := crypto.SignMessage(hashToSign[:])
	if err != nil {
		return err
	}
	fileManifestRequest := models.FileManifestRequest{
		Address:   config.AdvertisedAddress + ":" + strconv.Itoa(config.Port),
		PublicKey: config.PublicKey,
		Manifest:  *fileManifest,
		Signature: signature,
	}
	jsonBytesToSend, err := json.Marshal(fileManifestRequest)
	if err != nil {
		return err
	}
	resp, err := config.HttpClient.Post(fmt.Sprintf("http://%s/file/manifest", config.BootStrapServers[chosenServer]), "application/json", bytes.NewBuffer(jsonBytesToSend))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil
	}
	return fmt.Errorf("error with message: %v", resp.Body)
}

func pickRandomItems(originalList []string, n int) []string {
	list := make([]string, len(originalList))
	copy(list, originalList)
	selected := make([]string, 0, n)
	if n > len(list) {
		n = len(list)
	}
	for i := 0; i < n; i++ {
		randomIndex := rand.Intn(len(list))
		selected = append(selected, list[randomIndex])
		list[randomIndex] = list[len(list)-1]
		list = list[:len(list)-1]
	}
	return selected
}

func GetFileManifestFromServer(fileId string) (*models.FileManifest, error) {
	chosenServer := rand.Intn(len(config.BootStrapServers))
	request := models.GetFileManifestRequest{
		Address:   config.AdvertisedAddress + ":" + strconv.Itoa(config.Port),
		PublicKey: config.PublicKey,
		FileId:    fileId,
	}
	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	signature, err := crypto.SignMessage(jsonBytes)
	if err != nil {
		return nil, err
	}
	request.Signature = signature
	jsonBytes, err = json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := config.HttpClient.Post(fmt.Sprintf("http://%s/manifests", config.BootStrapServers[chosenServer]), "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var response *models.FileManifest
		json.NewDecoder(resp.Body).Decode(&response)
		return response, nil
	}
	return nil, fmt.Errorf("error with message: %v", resp.Body)
}

func SaveChunk(r *http.Request) error {
	var chunkRequest models.ChunkRequest
	err := json.NewDecoder(r.Body).Decode(&chunkRequest)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	message, err := json.Marshal(models.ChunkRequest{Address: chunkRequest.Address, PublicKey: chunkRequest.PublicKey, ChunkId: chunkRequest.ChunkId, Shard: chunkRequest.Shard, ReleaseDate: chunkRequest.ReleaseDate})
	if err != nil {
		return err
	}
	check, err := crypto.VerifySignature(message, chunkRequest.Signature, chunkRequest.PublicKey)
	if err != nil {
		return err
	}
	if check {
		payload, err := json.Marshal(models.ChunkRequest{PublicKey: chunkRequest.PublicKey, Address: chunkRequest.Address, ChunkId: chunkRequest.ChunkId, Shard: chunkRequest.Shard, ReleaseDate: chunkRequest.ReleaseDate})
		if err != nil {
			return err
		}
		return database.PutData(config.DB, "chunks", chunkRequest.ChunkId, payload)
	}
	return fmt.Errorf("signature verification failed")
}

func GetChunk(r *http.Request) ([]byte, error) {
	chunkId := r.URL.Query().Get("chunkId")
	if chunkId == "" {
		return nil, fmt.Errorf("error 'chunkId' empty")
	}
	defer r.Body.Close()
	return database.GetData(config.DB, "chunks", chunkId)
}

func RequestChunk(node string, chunkId string) (*models.ChunkRequest, error) {
	resp, err := config.HttpClient.Get(fmt.Sprintf("http://%s/chunk?chunkId=%s", node, chunkId))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var chunkRequest models.ChunkRequest
		err := json.NewDecoder(resp.Body).Decode(&chunkRequest)
		if err != nil {
			return nil, err
		}
		return &chunkRequest, nil
	}
	body, _ := io.ReadAll(resp.Body)
	return nil, fmt.Errorf("error: %s", string(body))
}

func RequestChunkDeletion(nodeAddress string, chunkId string) error {
	url := fmt.Sprintf("http://%s/chunk?id=%s", nodeAddress, chunkId)
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	client := &http.Client{Timeout: 5 * time.Second}
	_, err := client.Do(req)
	return err
}

func DeleteManifestOnBootstrap(fileId string) error {
	chosenServer := rand.Intn(len(config.BootStrapServers))
	request := models.GetFileManifestRequest{
		Address:   config.AdvertisedAddress + ":" + strconv.Itoa(config.Port),
		PublicKey: config.PublicKey,
		FileId:    fileId,
	}

	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}
	signature, err := crypto.SignMessage(jsonBytes)
	if err != nil {
		return err
	}
	request.Signature = signature

	jsonBytes, err = json.Marshal(request)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://%s/manifests", config.BootStrapServers[chosenServer]), bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := config.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("error deleting manifest: %s", string(body))
}
