# go-ethereum-server

## 1. Install Docker

## 2. Run Server

```
docker-compose --env-file config/.env.example up
```

## REST API

- Get the latest blocks
  [GET] http://localhost:8080/blocks?limit=n

- Get the block by block number
  [GET] http://localhost:8080/blocks/:num

- Get the transaction data with event logs
  [GET] http://localhost:8080/transaction/:txHash
