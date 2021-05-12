package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"Kumazan/go-ethereum-server/pkg/model"
)

type Repo interface {
	ListBlocks(fromNum, toNum uint64) ([]*model.Block, error)
	CreateBlocks(block ...*model.Block) error
	GetTransaction(txHash string) (*model.Transaction, error)
	CreateTransaction(tx *model.Transaction) error
	UpdateTransactionLogs(tx *model.Transaction) error

	GetBlockNumber(ctx context.Context) (uint64, error)
	SetBlockNumber(ctx context.Context, num uint64) error
	GetBlock(ctx context.Context, num uint64) (*model.Block, error)
	SetBlock(ctx context.Context, num uint64, block *model.Block) error

	LockBlockNumber(ctx context.Context) (bool, error)
	UnlockBlockNumber(ctx context.Context) error
	LockBlock(ctx context.Context, num uint64) (bool, error)
	UnlockBlock(ctx context.Context, num uint64) error
}

const (
	blockNumberCacheKey = "block-number"
	blockNumberLockKey  = "retrieve-block-number-lock"
	blockCacheKeyPrefix = "block:"
	blockLockKeyPrefix  = "retrieve-block-lock:"

	blockNumberCacheTTL = time.Second * 5
	blockNumberLockTTL  = time.Second * 3
	blockCacheTTL       = time.Hour
	blockLockTTL        = time.Second * 3
)

var (
	ErrNotFound = errors.New("not found")
)

type repo struct {
	db    *gorm.DB
	redis *redis.Client
}

func New(db *gorm.DB, redis *redis.Client) Repo {
	return &repo{db: db, redis: redis}
}

func (repo *repo) ListBlocks(fromNum, toNum uint64) ([]*model.Block, error) {
	blocks := make([]*model.Block, 0, toNum-fromNum)
	if err := repo.db.Where("block_num BETWEEN ? AND ?", fromNum, toNum).
		Order("block_num desc").Find(&blocks).Error; err != nil {
		return nil, err
	}
	return blocks, nil
}

func (repo *repo) CreateBlocks(block ...*model.Block) error {
	return repo.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&block).Error
}

func (repo *repo) GetTransaction(txHash string) (*model.Transaction, error) {
	var tx *model.Transaction
	err := repo.db.Where("tx_hash = ?", txHash).First(&tx).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (repo *repo) CreateTransaction(tx *model.Transaction) error {
	return repo.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&tx).Error
}

func (repo *repo) UpdateTransactionLogs(tx *model.Transaction) error {
	value, _ := tx.Logs.Value()
	return repo.db.Model(&tx).Update("logs", value).Error
}

func (repo *repo) GetBlockNumber(ctx context.Context) (uint64, error) {
	res, err := repo.redis.Get(ctx, blockNumberCacheKey).Result()
	if err == redis.Nil {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	blockNum, err := strconv.Atoi(res)
	if err != nil {
		return 0, err
	}
	return uint64(blockNum), nil
}

func (repo *repo) SetBlockNumber(ctx context.Context, num uint64) error {
	return repo.redis.Set(ctx, blockNumberCacheKey, num, blockNumberCacheTTL).Err()
}

func (repo *repo) LockBlockNumber(ctx context.Context) (bool, error) {
	getLock, err := repo.redis.SetNX(ctx, blockNumberLockKey, true, blockNumberLockTTL).Result()
	if err != nil {
		return false, err
	}
	return getLock, err
}

func (repo *repo) UnlockBlockNumber(ctx context.Context) error {
	return repo.redis.Del(ctx, blockNumberLockKey).Err()
}

func (repo *repo) GetBlock(ctx context.Context, num uint64) (*model.Block, error) {
	key := fmt.Sprintf("%s%d", blockCacheKeyPrefix, num)
	res, err := repo.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var block *model.Block
	err = json.Unmarshal([]byte(res), &block)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (repo *repo) SetBlock(ctx context.Context, num uint64, block *model.Block) error {
	key := fmt.Sprintf("%s%d", blockCacheKeyPrefix, num)
	return repo.redis.Set(ctx, key, block, blockCacheTTL).Err()
}

func (repo *repo) LockBlock(ctx context.Context, num uint64) (bool, error) {
	key := fmt.Sprintf("%s%d", blockLockKeyPrefix, num)
	getLock, err := repo.redis.SetNX(ctx, key, true, blockLockTTL).Result()
	if err != nil {
		return false, err
	}
	return getLock, err
}

func (repo *repo) UnlockBlock(ctx context.Context, num uint64) error {
	key := fmt.Sprintf("%s%d", blockLockKeyPrefix, num)
	return repo.redis.Del(ctx, key).Err()
}
