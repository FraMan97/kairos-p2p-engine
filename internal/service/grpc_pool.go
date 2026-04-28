package service

import (
	"fmt"
	"net"
	"strconv"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	grpcPool sync.Map
)

func GetGrpcConnection(nodeAddress string) (*grpc.ClientConn, error) {
	if connItem, ok := grpcPool.Load(nodeAddress); ok {
		conn := connItem.(*grpc.ClientConn)
		state := conn.GetState()
		if state != connectivity.Shutdown && state != connectivity.TransientFailure {
			return conn, nil
		}
		conn.Close()
		grpcPool.Delete(nodeAddress)
	}

	host, portStr, err := net.SplitHostPort(nodeAddress)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}

	grpcAddr := fmt.Sprintf("%s:%d", host, port+1000)

	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	grpcPool.Store(nodeAddress, conn)
	return conn, nil
}

func CloseAllGrpcConnections() {
	grpcPool.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*grpc.ClientConn); ok {
			conn.Close()
		}
		grpcPool.Delete(key)
		return true
	})
}
