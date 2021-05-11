package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"Kumazan/go-ethereum-server/pkg/model"
)

type EthereumService interface {
	ListLastestBlocks(ctx context.Context, limit int) ([]*model.Block, error)
	GetBlock(ctx context.Context, num uint64) (*model.Block, error)
	GetTransaction(ctx context.Context, txHash string) (*model.Transaction, error)
	RetrieveBlocks(ctx context.Context)
}

type service struct {
	ec    *ethclient.Client
	db    *gorm.DB
	redis *redis.Client
}

const rpcEndpoint = "https://data-seed-prebsc-2-s3.binance.org:8545"

func New(db *gorm.DB, redis *redis.Client) EthereumService {
	ec, err := ethclient.Dial(rpcEndpoint)
	if err != nil {
		log.Fatalf("ethclient.Dial failed: %+v", err)
	}
	return &service{ec: ec, db: db, redis: redis}
}

func (s *service) RetrieveBlocks(ctx context.Context) {
	const (
		dataKey = "block-number"
		dataTTL = time.Second * 3
	)

	limit := 0
	for range time.Tick(time.Second * 3) {
		if limit < 1000 {
			limit += 100
		}

		blockNumber, err := s.ec.BlockNumber(ctx)
		if err != nil {
			log.Printf("BlockNumber failed: %v\n", err)
			continue
		}
		err = s.redis.Set(ctx, dataKey, blockNumber, dataTTL).Err()
		if err != nil {
			log.Printf("redis.Set failed: %+v", err)
			continue
		}

		_, err = s.ListLastestBlocks(ctx, limit)
		if err != nil {
			log.Printf("ListLastestBlocks failed: %v\n", err)
			continue
		}
	}
}

func (s *service) ListLastestBlocks(ctx context.Context, limit int) ([]*model.Block, error) {
	blockNumber, err := s.RetrieveBlockNumber(ctx)
	if err != nil {
		return nil, err
	}
	fromNumber := blockNumber - uint64(limit) + 1
	toNumber := blockNumber

	savedBlocks := make([]*model.Block, 0, limit)
	s.db.Where("block_num BETWEEN ? AND ?", fromNumber, toNumber).
		Order("block_num desc").Find(&savedBlocks)
	if len(savedBlocks) == limit {
		return savedBlocks, nil
	}

	var wg sync.WaitGroup

	blocks := make([]*model.Block, limit)

	newCount := 0
	newBlocks := make(chan *model.Block, limit)
	var index int
	for num := toNumber; num >= fromNumber; num-- {
		if index < len(savedBlocks) && savedBlocks[index].BlockNum == num {
			blocks[toNumber-num] = savedBlocks[index]
			index++
			continue
		}

		wg.Add(1)
		go func(num uint64) {
			defer wg.Done()
			block, isNew, err := s.RetrieveBlock(ctx, num)
			if err != nil {
				log.Printf("BlockByNumber failed: %+v", err)
				return
			}
			blocks[toNumber-num] = block
			if isNew {
				newCount++
				newBlocks <- block
			}
		}(num)
	}
	wg.Wait()
	close(newBlocks)

	blocksToCreate := make([]*model.Block, 0, newCount)
	for b := range newBlocks {
		blocksToCreate = append(blocksToCreate, b)
	}
	err = s.db.Clauses(clause.OnConflict{UpdateAll: true}).
		Create(blocksToCreate).Error
	if err != nil {
		log.Printf("db.Create failed: %+v", err)
		return nil, err
	}
	// log.Printf("%d new blocks created", newCount)

	return blocks, nil
}

func (s *service) GetBlock(ctx context.Context, num uint64) (*model.Block, error) {
	block, isNew, err := s.RetrieveBlock(ctx, num)
	if err != nil {
		log.Printf("RetrieveBlock failed: %+v", err)
		return nil, err
	}
	if isNew {
		s.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&block)
	}

	block.TxHash = make([]string, len(block.Transactions))
	for i := range block.Transactions {
		block.TxHash[i] = block.Transactions[i].TxHash
	}
	return block, nil
}

func (s *service) RetrieveBlockNumber(ctx context.Context) (uint64, error) {
	const (
		dataTTL = time.Second * 3
		lockTTL = time.Second * 3
	)
	var (
		dataKey = "block-number"
		lockKey = "retrieve-block-number-lock"
	)
	res, err := s.redis.Get(ctx, dataKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("redis.Get failed: %+v", err)
		return 0, err
	}
	if len(res) > 0 {
		blockNum, err := strconv.Atoi(res)
		if err != nil {
			log.Printf("strconv.Atoi %v failed: %+v", res, err)
			return 0, err
		}
		return uint64(blockNum), nil
	}

	for {
		getLock, err := s.redis.SetNX(ctx, lockKey, true, lockTTL).Result()
		if err != nil {
			log.Printf("redis.SetNX failed: %+v", err)
			return 0, err
		}

		if !getLock {
			continue
		}
		defer func() {
			s.redis.Del(ctx, lockKey)
		}()

		res, err := s.redis.Get(ctx, dataKey).Result()
		if err != nil && err != redis.Nil {
			log.Printf("redis.Get failed: %+v", err)
			return 0, err
		}
		if len(res) > 0 {
			blockNum, err := strconv.Atoi(res)
			if err != nil {
				log.Printf("strconv.Atoi %v failed: %+v", res, err)
				return 0, err
			}
			return uint64(blockNum), nil
		}
		break
	}

	blockNumber, err := s.ec.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	err = s.redis.Set(ctx, dataKey, blockNumber, dataTTL).Err()
	if err != nil {
		log.Printf("redis.Set failed: %+v", err)
		return 0, err
	}
	return blockNumber, nil
}

func (s *service) RetrieveBlock(ctx context.Context, num uint64) (*model.Block, bool, error) {
	const lockTTL = time.Second * 3
	var (
		dataKey = fmt.Sprintf("block:%d", num)
		lockKey = fmt.Sprintf("retrieve-block-lock:%d", num)
	)
	res, err := s.redis.Get(ctx, dataKey).Result()
	if err != nil && err != redis.Nil {
		log.Printf("redis.Get failed: %+v", err)
		return nil, false, err
	}
	if len(res) > 0 {
		var block *model.Block
		err := json.Unmarshal([]byte(res), &block)
		if err != nil {
			log.Printf("json.Unmarshal %v failed: %+v", res, err)
			return nil, false, err
		}
		return block, false, nil
	}

	for {
		getLock, err := s.redis.SetNX(ctx, lockKey, 1, lockTTL).Result()
		if err != nil {
			log.Printf("redis.SetNX failed: %+v", err)
			return nil, false, err
		}

		if !getLock {
			continue
		}
		defer func() {
			s.redis.Del(ctx, lockKey)
		}()

		res, err := s.redis.Get(ctx, dataKey).Result()
		if err != nil && err != redis.Nil {
			log.Printf("redis.Get failed: %+v", err)
			return nil, false, err
		}
		if len(res) > 0 {
			var block *model.Block
			err := json.Unmarshal([]byte(res), &block)
			if err != nil {
				log.Printf("json.Unmarshal %v failed: %+v", res, err)
				return nil, false, err
			}
			return block, false, nil
		}

		break
	}

	b, err := s.ec.BlockByNumber(ctx, big.NewInt(int64(num)))
	if err != nil {
		log.Printf("BlockByNumber failed: %+v", err)
		return nil, false, err
	}
	block := model.NewBlock(b)
	err = s.redis.Set(ctx, dataKey, block, time.Hour).Err()
	if err != nil {
		log.Printf("redis.Set failed: %+v", err)
		return nil, false, err
	}
	return block, true, nil
}

func (s *service) GetTransaction(ctx context.Context, txHash string) (*model.Transaction, error) {
	var tx *model.Transaction

	err := s.db.Where("tx_hash = ?", txHash).First(&tx).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Printf("db.First failed: %+v", err)
			return nil, err
		}

		txn, _, err := s.ec.TransactionByHash(ctx, common.HexToHash(txHash))
		if err != nil {
			log.Printf("TransactionByHash failed: %+v", err)
			return nil, err
		}
		tx = model.NewTransaction(txn)

		s.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&tx)
	}
	if len(tx.Logs) == 0 {
		receipt, err := s.ec.TransactionReceipt(ctx, common.HexToHash(txHash))
		if err != nil {
			log.Printf("TransactionReceipt failed: %+v", err)
			return nil, err
		}
		tx.Logs = make([]model.Log, len(receipt.Logs))
		for i, log := range receipt.Logs {
			tx.Logs[i] = model.Log{
				Index: log.Index,
				Data:  common.BytesToHash(log.Data).String(),
			}
		}
		value, _ := tx.Logs.Value()
		if err := s.db.Model(&tx).Update("logs", value).Error; err != nil {
			log.Printf("Update tx.Logs failed: %+v, logs = %+v", err, tx.Logs)
			return nil, err
		}
	}

	return tx, nil
}
