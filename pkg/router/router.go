package router

import (
	"context"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"Kumazan/go-ethereum-server/pb"
	"Kumazan/go-ethereum-server/pkg/grpc"
	"Kumazan/go-ethereum-server/pkg/model"
)

type Handler struct {
	*gin.Engine
	ctx context.Context
	ec  *grpc.EthereumClient
}

func New(ec *grpc.EthereumClient) Handler {
	h := Handler{
		Engine: gin.Default(),
		ctx:    context.Background(),
		ec:     ec,
	}

	h.GET("/blocks", h.listBlocks)
	h.GET("/blocks/:id", h.getBlock)
	h.GET("/transaction/:txHash", h.getTransaction)

	return h
}

func (h *Handler) listBlocks(c *gin.Context) {
	qLimit := c.DefaultQuery("limit", "1")
	limit, err := strconv.Atoi(qLimit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "limit is not a number",
		})
		return
	}
	if limit < 0 || limit > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "limit is invalid",
		})
		return
	}

	req := &pb.ListLastestBlocksRequest{Limit: int32(limit)}
	resp, err := h.ec.ListLastestBlocks(h.ctx, req)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"blocks": resp.Blocks,
	})
}

func (h *Handler) getBlock(c *gin.Context) {
	pNum := c.Param("id")
	blockNum, err := strconv.Atoi(pNum)
	if err != nil || blockNum < 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "id is invalid",
		})
		return
	}

	req := &pb.GetBlockRequest{BlockNum: int64(blockNum)}
	resp, err := h.ec.GetBlock(h.ctx, req)
	if err != nil {
		status, ok := status.FromError(err)
		if ok && status.Code() == codes.NotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"message": status.Message(),
			})
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, resp.Block)
}

var hashValidator = regexp.MustCompile(`^0x([A-Fa-f0-9]{64})$`)

func (h *Handler) getTransaction(c *gin.Context) {
	txHash := c.Param("txHash")
	if !hashValidator.MatchString(txHash) {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "txHash is invalid",
		})
		return
	}

	req := &pb.GetTransactionRequest{TxHash: txHash}
	resp, err := h.ec.GetTransaction(h.ctx, req)
	if err != nil {
		status, ok := status.FromError(err)
		if ok && status.Code() == codes.NotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"message": status.Message(),
			})
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}

	tx := resp.Tx
	logs := make([]model.Log, len(tx.Logs))
	for i, log := range tx.Logs {
		logs[i] = model.Log{
			Index: uint(log.Index),
			Data:  log.Data,
		}
	}
	c.JSON(http.StatusOK, model.Transaction{
		TxHash:   tx.TxHash,
		FromAddr: tx.FromAddr,
		ToAddr:   tx.ToAddr,
		Nonce:    uint64(tx.Nonce),
		Data:     tx.Data,
		Value:    tx.Value,
		Logs:     logs,
	})
}
