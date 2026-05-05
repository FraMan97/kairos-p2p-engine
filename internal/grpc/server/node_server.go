package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/FraMan97/kairos-p2p-engine/internal/crypto"
	"github.com/FraMan97/kairos-p2p-engine/internal/database"
	"github.com/FraMan97/kairos-p2p-engine/internal/grpc/pb"
	"github.com/FraMan97/kairos-p2p-engine/internal/models"
)

type NodeServer struct {
	pb.UnimplementedNodeServiceServer
}

func (s *NodeServer) SaveChunk(ctx context.Context, req *pb.ChunkRequest) (*pb.ChunkResponse, error) {
	message, err := json.Marshal(models.ChunkRequest{
		Address:     req.Address,
		PublicKey:   req.PublicKey,
		ChunkId:     req.ChunkId,
		FileId:      req.FileId,
		CreatedAt:   req.CreatedAt,
		Shard:       req.Shard,
		ReleaseDate: req.ReleaseDate,
	})
	if err != nil {
		return &pb.ChunkResponse{Success: false, Message: "serialization error"}, err
	}

	check, err := crypto.VerifySignature(message, req.Signature, req.PublicKey)
	if err != nil || !check {
		return &pb.ChunkResponse{Success: false, Message: "unauthorized"}, fmt.Errorf("signature verification failed")
	}

	payload, err := json.Marshal(models.ChunkRequest{
		PublicKey:   req.PublicKey,
		Address:     req.Address,
		ChunkId:     req.ChunkId,
		FileId:      req.FileId,
		CreatedAt:   req.CreatedAt,
		Shard:       req.Shard,
		ReleaseDate: req.ReleaseDate,
	})
	if err != nil {
		return &pb.ChunkResponse{Success: false, Message: "payload creation error"}, err
	}

	err = database.PutData(config.DB, "chunks", req.ChunkId, payload)
	if err != nil {
		return &pb.ChunkResponse{Success: false, Message: "database error"}, err
	}

	return &pb.ChunkResponse{Success: true, Message: "chunk saved"}, nil
}

func (s *NodeServer) GetChunk(ctx context.Context, req *pb.GetChunkRequest) (*pb.ChunkData, error) {
	data, err := database.GetData(config.DB, "chunks", req.ChunkId)
	if err != nil {
		return nil, fmt.Errorf("chunk not found")
	}

	return &pb.ChunkData{Data: data}, nil
}

func (s *NodeServer) DeleteChunk(ctx context.Context, req *pb.DeleteChunkRequest) (*pb.ChunkResponse, error) {
	err := database.DeleteKey(config.DB, "chunks", req.ChunkId)
	if err != nil {
		return &pb.ChunkResponse{Success: false, Message: "failed to delete chunk"}, err
	}

	return &pb.ChunkResponse{Success: true, Message: "chunk deleted"}, nil
}