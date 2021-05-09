package grpc

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"

	"Kumazan/go-ethereum-server/pb"
)

type EthereumClient struct {
	*grpc.ClientConn
	pb.EthereumServiceClient
}

const (
	address = "indexer:5001"
)

func NewClient() *EthereumClient {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address,
		grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("grpc.DialContext failed: %v", err)
	}
	return &EthereumClient{
		ClientConn:            conn,
		EthereumServiceClient: pb.NewEthereumServiceClient(conn),
	}
}
