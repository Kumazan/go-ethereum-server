package main

import (
	"log"

	"Kumazan/go-ethereum-server/pkg/grpc"
	"Kumazan/go-ethereum-server/pkg/router"
)

func main() {
	grpcClient := grpc.NewClient()
	router := router.New(grpcClient)
	if err := router.Engine.Run(); err != nil {
		log.Fatalf("failed to run: %v", err)
	}
}
