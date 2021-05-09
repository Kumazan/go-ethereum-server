package main

import (
	"log"
	"net"

	"Kumazan/go-ethereum-server/db"
	"Kumazan/go-ethereum-server/pkg/grpc"
	"Kumazan/go-ethereum-server/pkg/service"
)

const (
	port = ":5001"
)

func main() {
	service := service.New(db.New())
	server := grpc.NewServer(service)

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
