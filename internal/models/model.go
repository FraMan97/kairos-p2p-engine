package models

type SubscriptionRequest struct {
	Address   string `json:"address"`
	PublicKey []byte `json:"public_key"`
	Signature []byte `json:"signature"`
}

type SynchronizationRequest struct {
	Address       string            `json:"address"`
	PublicKey     []byte            `json:"public_key"`
	ActiveNodes   map[string][]byte `json:"active_nodes"`
	FileManifests map[string][]byte `json:"chunks"`
	Signature     []byte            `json:"signature"`
}

type NodesForFileUploadRequest struct {
	Address       string `json:"address"`
	PublicKey     []byte `json:"public_key"`
	Signature     []byte `json:"signature"`
	TotalChunks   int    `json:"total_chunks"`
	NodesPerChunk int    `json:"nodes_per_chunk"`
}

type ActiveNodeRecord struct {
	PublicKey []byte
	Timestamp int64
}

type FileManifestRequest struct {
	Address   string       `json:"address"`
	PublicKey []byte       `json:"public_key"`
	Signature []byte       `json:"signature"`
	Manifest  FileManifest `json:"manifest"`
}

type GetFileManifestRequest struct {
	Address   string `json:"address"`
	PublicKey []byte `json:"public_key"`
	Signature []byte `json:"signature"`
	FileId    string `json:"file_id"`
}

type ChunkRequest struct {
	Address     string `json:"address"`
	PublicKey   []byte `json:"public_key"`
	Signature   []byte `json:"signature"`
	ChunkId     string `json:"chunk_id"`
	Shard       []byte `json:"shard"`
	ReleaseDate string `json:"release_date"`
}

type FileManifest struct {
	FileName          string            `json:"file_name"`
	FileId            string            `json:"file_id"`
	FileSize          int64             `json:"file_size"`
	ReleaseDate       string            `json:"release_date"`
	HashFile          string            `json:"hash_file"`
	HashAlgorithm     string            `json:"hash_algorithm"`
	Blocks            int               `json:"blocks"`
	ChunksPerBlocks   int               `json:"chunks_per_blocks"`
	ReedSolomonConfig ReedSolomonConfig `json:"reed_solomon_config"`
	Split             map[int]FileBlock `json:"split"`
}

type ReedSolomonConfig struct {
	DataShards   int `json:"data_shards"`
	ParityShards int `json:"parity_shards"`
}

type FileBlock struct {
	EncryptedBlockSize int     `json:"encrypted_block_size"`
	Chunks             []Chunk `json:"chunks"`
}

type Chunk struct {
	ChunkId      string   `json:"chunk_id"`
	KeyIndexPart byte     `json:"key_index_part"`
	KeyPart      []byte   `json:"key_part"`
	ShardIndex   int      `json:"shard_index"`
	Nodes        []string `json:"nodes"`
}
