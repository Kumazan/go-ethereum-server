package service

import (
	"context"
	"log"
	"math/big"
	"os"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"Kumazan/go-ethereum-server/pkg/model"
)

type EthereumService interface {
	ListLastestBlocks(ctx context.Context, limit int) ([]*model.Block, error)
	GetBlock(ctx context.Context, num uint64) (*model.Block, error)
	GetTransaction(ctx context.Context, txHash string) (*model.Transaction, error)
}

type service struct {
	ec *ethclient.Client
	db *gorm.DB
}

var rpcEndpoint = os.Getenv("RPC_ENDPOINT")

func New(db *gorm.DB) EthereumService {
	ec, err := ethclient.Dial(rpcEndpoint)
	if err != nil {
		log.Fatalf("ethclient.Dial failed: %+v", err)
	}
	return &service{ec: ec, db: db}
}

func (s *service) ListLastestBlocks(ctx context.Context, limit int) ([]*model.Block, error) {
	blockNumber, err := s.ec.BlockNumber(ctx)
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

	blocksToCreate := make([]*model.Block, 0, limit)
	var index int
	for num := toNumber; num >= fromNumber; num-- {
		if index < len(savedBlocks) && savedBlocks[index].BlockNum == num {
			index++
			continue
		}
		wg.Add(1)
		go func(num uint64) {
			defer wg.Done()
			block, err := s.ec.BlockByNumber(ctx, big.NewInt(int64(num)))
			if err != nil {
				log.Printf("BlockByNumber failed: %+v", err)
				return
			}
			blocksToCreate = append(blocksToCreate, model.NewBlock(block))
		}(num)
	}
	wg.Wait()

	err = s.db.Clauses(clause.OnConflict{UpdateAll: true}).
		Create(blocksToCreate).Error
	if err != nil {
		log.Printf("db.Create failed: %+v", err)
		return nil, err
	}

	blocks := make([]*model.Block, limit)
	err = s.db.Order("block_num desc").Limit(limit).Find(&blocks).Error
	if err != nil {
		log.Printf("db.Find failed: %+v", err)
		return nil, err
	}
	return blocks, nil
}

func (s *service) GetBlock(ctx context.Context, num uint64) (*model.Block, error) {
	var block *model.Block

	err := s.db.Where("block_num = ?", num).Preload("Transactions").First(&block).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Printf("db.First failed: %+v", err)
			return nil, err
		}

		b, err := s.ec.BlockByNumber(ctx, big.NewInt(int64(num)))
		if err != nil {
			log.Printf("BlockByNumber failed: %+v", err)
			return nil, err
		}
		block = model.NewBlock(b)
		s.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&block)
	}

	block.TxHash = make([]string, len(block.Transactions))
	for i := range block.Transactions {
		block.TxHash[i] = block.Transactions[i].TxHash
	}
	return block, nil
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
