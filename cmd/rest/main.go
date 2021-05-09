package main

import (
	"Kumazan/go-ethereum-server/db"
	"Kumazan/go-ethereum-server/pkg/router"
	"Kumazan/go-ethereum-server/pkg/service"
)

func main() {
	service := service.New(db.New())
	router := router.New(service)
	router.Run()
}
