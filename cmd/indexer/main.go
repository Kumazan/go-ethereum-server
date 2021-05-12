package main

import (
	"context"
	"log"
	"net"

	"Kumazan/go-ethereum-server/db"
	"Kumazan/go-ethereum-server/pkg/grpc"
	"Kumazan/go-ethereum-server/pkg/repo"
	"Kumazan/go-ethereum-server/pkg/service"
	"Kumazan/go-ethereum-server/redis"
)

const (
	port = ":5001"
)

func main() {
	repo := repo.New(db.New(), redis.NewClient())
	service := service.New(repo)
	go func() {
		service.RetrieveBlocks(context.Background())
	}()
	server := grpc.NewServer(service)

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
