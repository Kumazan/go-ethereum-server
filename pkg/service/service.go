package service

import (
	"context"
	"errors"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"Kumazan/go-ethereum-server/pkg/model"
	"Kumazan/go-ethereum-server/pkg/repo"
)

type EthereumService interface {
	ListLastestBlocks(ctx context.Context, limit int) ([]*model.Block, error)
	GetBlock(ctx context.Context, num uint64) (*model.Block, error)
	GetTransaction(ctx context.Context, txHash string) (*model.Transaction, error)
	RetrieveBlocks(ctx context.Context)
}

var ErrNotFound = errors.New("not found")

type service struct {
	ec   *ethclient.Client
	repo repo.Repo
}

const rpcEndpoint = "https://data-seed-prebsc-2-s3.binance.org:8545"

func New(repo repo.Repo) EthereumService {
	ec, err := ethclient.Dial(rpcEndpoint)
	if err != nil {
		log.Fatalf("ethclient.Dial failed: %+v", err)
	}
	return &service{ec: ec, repo: repo}
}

func (s *service) RetrieveBlocks(ctx context.Context) {

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
		err = s.repo.SetBlockNumber(ctx, blockNumber)
		if err != nil {
			log.Printf("repo.SetBlockNumber failed: %+v", err)
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

	savedBlocks, err := s.repo.ListBlocks(fromNumber, toNumber)
	if err != nil {
		log.Printf("repo.ListBlocks failed: %v", err)
		return nil, err
	}
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
	err = s.repo.CreateBlocks(blocksToCreate...)
	if err != nil {
		log.Printf("db.Create failed: %+v", err)
		return nil, err
	}
	// log.Printf("%d new blocks created", newCount)

	return blocks, nil
}

func (s *service) GetBlock(ctx context.Context, num uint64) (*model.Block, error) {
	currentNum, err := s.RetrieveBlockNumber(ctx)
	if err != nil {
		log.Printf("repo.GetBlockNumber failed: %+v", err)
	} else if num > currentNum {
		return nil, ErrNotFound
	}

	block, isNew, err := s.RetrieveBlock(ctx, num)
	if err != nil {
		log.Printf("RetrieveBlock failed: %+v", err)
		return nil, err
	}
	if isNew {
		err = s.repo.CreateBlocks(block)
		if err != nil {
			log.Printf("repo.CreateBlock failed: %+v", err)
		}
	}
	return block, nil
}

func (s *service) RetrieveBlockNumber(ctx context.Context) (uint64, error) {
	num, err := s.repo.GetBlockNumber(ctx)
	if err != nil && err != repo.ErrNotFound {
		log.Printf("repo.GetBlockNumber failed: %+v", err)
		return 0, err
	}
	if err == nil {
		return num, nil
	}

	for {
		getLock, err := s.repo.LockBlockNumber(ctx)
		if err != nil {
			log.Printf("repo.LockBlockNumber failed: %+v", err)
			return 0, err
		}

		if !getLock {
			continue
		}
		defer func() {
			err := s.repo.UnlockBlockNumber(ctx)
			if err != nil {
				log.Printf("repo.UnlockBlockNumber failed: %+v", err)
			}
		}()

		num, err := s.repo.GetBlockNumber(ctx)
		if err != nil && err != repo.ErrNotFound {
			log.Printf("repo.GetBlockNumber failed: %+v", err)
			return 0, err
		}
		if err == nil {
			return num, nil
		}
		break
	}

	blockNumber, err := s.ec.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	err = s.repo.SetBlockNumber(ctx, blockNumber)
	if err != nil {
		log.Printf("repo.SetBlockNumber failed: %+v", err)
		return 0, err
	}
	return blockNumber, nil
}

func (s *service) RetrieveBlock(ctx context.Context, num uint64) (*model.Block, bool, error) {
	block, err := s.repo.GetBlockCache(ctx, num)
	if err != nil && err != repo.ErrNotFound {
		log.Printf("repo.GetBlockCache failed: %+v", err)
		return nil, false, err
	}
	if err == nil {
		return block, false, nil
	}

	for {
		getLock, err := s.repo.LockBlock(ctx, num)
		if err != nil {
			log.Printf("repo.LockBlock failed: %+v", err)
			return nil, false, err
		}

		if !getLock {
			continue
		}
		defer func() {
			err := s.repo.UnlockBlock(ctx, num)
			if err != nil {
				log.Printf("repo.UnlockBlockNumber failed: %+v", err)
			}
		}()

		block, err := s.repo.GetBlockCache(ctx, num)
		if err != nil && err != repo.ErrNotFound {
			log.Printf("repo.GetBlockCache failed: %+v", err)
			return nil, false, err
		}
		if err == nil {
			return block, false, nil
		}

		break
	}

	b, err := s.ec.BlockByNumber(ctx, big.NewInt(int64(num)))
	if err != nil {
		if err == ethereum.NotFound {
			return nil, false, ErrNotFound
		}
		log.Printf("BlockByNumber failed: %+v", err)
		return nil, false, err
	}
	block = model.NewBlock(b)
	block.TxHash = make([]string, len(block.Transactions))
	for i := range block.Transactions {
		block.TxHash[i] = block.Transactions[i].TxHash
	}
	err = s.repo.SetBlockCache(ctx, num, block)
	if err != nil {
		log.Printf("repo.SetBlockCache failed: %+v", err)
	}
	return block, true, nil
}

func (s *service) GetTransaction(ctx context.Context, txHash string) (*model.Transaction, error) {
	tx, err := s.repo.GetTxCache(ctx, txHash)
	if err == nil {
		return tx, nil
	}
	if err != nil && err != repo.ErrNotFound {
		log.Printf("repo.GetTxCache failed: %+v", err)
		return nil, err
	}

	tx, err = s.repo.GetTransaction(txHash)
	if err != nil && err != repo.ErrNotFound {
		log.Printf("repo.GetTransaction failed: %+v", err)
		return nil, err
	}
	if err == repo.ErrNotFound {
		txn, _, err := s.ec.TransactionByHash(ctx, common.HexToHash(txHash))
		if err != nil {
			if err == ethereum.NotFound {
				// TODO: cache not found
				return nil, ErrNotFound
			}
			log.Printf("TransactionByHash failed: %+v", err)
			return nil, err
		}
		tx = model.NewTransaction(txn)
		if err := s.repo.CreateTransaction(tx); err != nil {
			log.Printf("repo.CreateTransaction failed: %v", err)
		}
	}

	if tx.Logs == nil {
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
		if err := s.repo.UpdateTransactionLogs(tx); err != nil {
			log.Printf("repo.UpdateTransactionLogs failed: %v", err)
		}
	}

	if err := s.repo.SetTxCache(ctx, txHash, tx); err != nil {
		log.Printf("repo.SetTxCache failed: %v", err)
	}

	return tx, nil
}
