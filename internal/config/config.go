package config

import (
	"net/http"
	"time"

	"github.com/dgraph-io/badger/v4"
)

var (
	DB *badger.DB

	AdvertisedAddress string
	Port              int = 8080
	HttpClient        *http.Client
	BootStrapServers  []string

	PublicKey     []byte
	PrivateKey    []byte
	PublicKeyDir  string
	PrivateKeyDir string

	TargetChunkSize int = 500 * 1024
	DataShards      int = 3
	ParityShards    int = 2
	TotalShards     int = DataShards + ParityShards
	ChunksTolerance int = 3
	DrandChainHash      = "52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971"
	DrandRelays         = []string{"https://api.drand.sh", "https://drand.cloudflare.com"}

	CronSync  int = 10
	CronClean int = 3600
)

func InitHttpClient() {
	HttpClient = &http.Client{
		Timeout: 30 * time.Second,
	}
}
