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
	CreateBlocks(block ...*model.Block) error
	GetTransaction(txHash string) (*model.Transaction, error)
	CreateTransaction(tx *model.Transaction) error
	UpdateTransactionLogs(tx *model.Transaction) error

	ListBlocks(ctx context.Context, fromNum, toNum uint64) ([]*model.Block, error)
	GetBlockNumber(ctx context.Context) (uint64, error)
	SetBlockNumber(ctx context.Context, num uint64) error
	GetBlockCache(ctx context.Context, num uint64) (*model.Block, error)
	SetBlockCache(ctx context.Context, block ...*model.Block) error
	DelBlockCache(ctx context.Context, block ...*model.Block) error
	GetTxCache(ctx context.Context, txHash string) (*model.Transaction, error)
	SetTxCache(ctx context.Context, txHash string, tx *model.Transaction) error

	LockBlockNumber(ctx context.Context) (bool, error)
	UnlockBlockNumber(ctx context.Context) error
	LockBlock(ctx context.Context, num uint64) (bool, error)
	UnlockBlock(ctx context.Context, num uint64) error
	LockTransaction(ctx context.Context, txHash string) (bool, error)
	UnlockTransaction(ctx context.Context, txHash string) error
}

const (
	blockNumberCacheKey = "block-number"
	blockNumberLockKey  = "retrieve-block-number-lock"
	blockListCacheKey   = "blocks"
	blockCacheKeyPrefix = "block:"
	blockLockKeyPrefix  = "retrieve-block-lock:"
	txCacheKeyPrefix    = "transaction:"
	txLockKeyPrefix     = "transaction-lock:"

	blockNumberCacheTTL = time.Second * 5
	blockNumberLockTTL  = time.Second * 3
	blockLockTTL        = time.Second * 3
	txCacheTTL          = time.Hour
	txLockTTL           = time.Second * 3
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

func (repo *repo) ListBlocks(ctx context.Context, fromNum, toNum uint64) ([]*model.Block, error) {
	zset, err := repo.redis.ZRevRangeByScore(ctx, blockListCacheKey,
		&redis.ZRangeBy{
			Min: fmt.Sprint(fromNum),
			Max: fmt.Sprint(toNum),
		}).Result()
	if err != nil {
		return nil, err
	}

	blocks := make([]*model.Block, len(zset))
	for i, b := range zset {
		var block *model.Block
		err := json.Unmarshal([]byte(b), &block)
		if err != nil {
			return nil, err
		}
		blocks[i] = block
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

func (repo *repo) GetBlockCache(ctx context.Context, num uint64) (*model.Block, error) {
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

func (repo *repo) SetBlockCache(ctx context.Context, blocks ...*model.Block) error {
	msetValues := make(map[string]interface{}, len(blocks))
	zmembers := make([]*redis.Z, len(blocks))
	for i, block := range blocks {
		num := block.BlockNum
		key := fmt.Sprintf("%s%d", blockCacheKeyPrefix, num)
		value, _ := block.MarshalBinary()
		msetValues[key] = value
		zmembers[i] = &redis.Z{
			Score:  float64(num),
			Member: block,
		}
	}
	err := repo.redis.MSet(ctx, msetValues).Err()
	if err != nil {
		return err
	}
	return repo.redis.ZAdd(ctx, blockListCacheKey, zmembers...).Err()
}

func (repo *repo) DelBlockCache(ctx context.Context, blocks ...*model.Block) error {
	delKeys := make([]string, len(blocks))
	zmembers := make([]interface{}, len(blocks))
	for i, block := range blocks {
		delKeys[i] = fmt.Sprintf("%s%d", blockCacheKeyPrefix, block.BlockNum)
		zmembers[i] = block
	}
	err := repo.redis.Del(ctx, delKeys...).Err()
	if err != nil {
		return err
	}
	return repo.redis.ZRem(ctx, blockListCacheKey, zmembers...).Err()
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

func (repo *repo) GetTxCache(ctx context.Context, txHash string) (*model.Transaction, error) {
	key := fmt.Sprintf("%s%s", txCacheKeyPrefix, txHash)
	res, err := repo.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var tx *model.Transaction
	err = json.Unmarshal([]byte(res), &tx)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (repo *repo) SetTxCache(ctx context.Context, txHash string, tx *model.Transaction) error {
	key := fmt.Sprintf("%s%s", txCacheKeyPrefix, txHash)
	return repo.redis.Set(ctx, key, tx, txCacheTTL).Err()
}

func (repo *repo) LockTransaction(ctx context.Context, txHash string) (bool, error) {
	key := fmt.Sprintf("%s%s", txLockKeyPrefix, txHash)
	getLock, err := repo.redis.SetNX(ctx, key, true, blockLockTTL).Result()
	if err != nil {
		return false, err
	}
	return getLock, err
}

func (repo *repo) UnlockTransaction(ctx context.Context, txHash string) error {
	key := fmt.Sprintf("%s%s", txLockKeyPrefix, txHash)
	return repo.redis.Del(ctx, key).Err()
}
