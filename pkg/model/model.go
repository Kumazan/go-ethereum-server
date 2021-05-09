package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type Block struct {
	BlockNum     uint64         `json:"block_num" gorm:"primaryKey"`
	BlockHash    string         `json:"block_hash"`
	BlockTime    uint64         `json:"block_time"`
	ParentHash   string         `json:"parent_hash"`
	Transactions []*Transaction `json:"-" gorm:"foreignKey:BlockNum;references:BlockNum"`
	TxHash       []string       `json:"transactions,omitempty" gorm:"-"`
}

func NewBlock(b *types.Block) *Block {
	header := b.Header()
	blockNum := header.Number.Uint64()
	txns := make([]*Transaction, len(b.Transactions()))
	for i, tx := range b.Transactions() {
		txns[i] = NewTransaction(tx)
		txns[i].BlockNum = blockNum
	}
	return &Block{
		BlockNum:     blockNum,
		BlockHash:    header.Hash().String(),
		BlockTime:    header.Time,
		ParentHash:   header.ParentHash.String(),
		Transactions: txns,
	}
}

type Transaction struct {
	TxHash   string `json:"tx_hash" gorm:"primaryKey"`
	BlockNum uint64 `json:"-"`
	FromAddr string `json:"from"`
	ToAddr   string `json:"to"`
	Nonce    uint64 `json:"nonce"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	Logs     Logs   `json:"logs"`
}

type Logs []Log
type Log struct {
	Index uint   `json:"index"`
	Data  string `json:"data"`
}

func (l *Logs) Value() (driver.Value, error) {
	return json.Marshal(l)
}

func (l *Logs) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &l)
}

func NewTransaction(tx *types.Transaction) *Transaction {
	msg, _ := tx.AsMessage(types.NewEIP155Signer(tx.ChainId()))
	var toAddr string
	if to := tx.To(); to != nil {
		toAddr = to.String()
	}
	data := common.BytesToHash(tx.Data())
	return &Transaction{
		TxHash:   tx.Hash().String(),
		FromAddr: msg.From().String(),
		ToAddr:   toAddr,
		Nonce:    tx.Nonce(),
		Data:     data.String(),
		Value:    tx.Value().String(),
	}
}
