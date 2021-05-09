package router

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/ethereum/go-ethereum"
	"github.com/gin-gonic/gin"

	"Kumazan/go-ethereum-server/pkg/service"
)

type Handler struct {
	*gin.Engine

	es service.EthereumService
}

func New(es service.EthereumService) Handler {
	h := Handler{
		Engine: gin.Default(),
		es:     es,
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
	if limit < 0 || limit > 1024 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "limit is invalid",
		})
		return
	}

	ctx := context.Background()
	blocks, err := h.es.ListLastestBlocks(ctx, limit)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"blocks": blocks,
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

	ctx := context.Background()
	block, err := h.es.GetBlock(ctx, uint64(blockNum))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "block not found",
			})
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, block)
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

	ctx := context.Background()
	tx, err := h.es.GetTransaction(ctx, txHash)
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "txHash not found",
			})
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, tx)
}
