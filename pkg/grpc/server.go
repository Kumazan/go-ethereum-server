package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"Kumazan/go-ethereum-server/pb"
	"Kumazan/go-ethereum-server/pkg/service"
)

type EthereumServer struct {
	*grpc.Server

	svc service.EthereumService
}

func NewServer(svc service.EthereumService) *EthereumServer {
	grpcServer := grpc.NewServer()
	s := &EthereumServer{Server: grpcServer, svc: svc}
	pb.RegisterEthereumServiceServer(grpcServer, s)
	return s
}

func (s *EthereumServer) ListLastestBlocks(ctx context.Context, req *pb.ListLastestBlocksRequest) (*pb.ListLastestBlocksResponse, error) {
	blocks, err := s.svc.ListLastestBlocks(ctx, int(req.Limit))
	if err != nil {
		return &pb.ListLastestBlocksResponse{}, err
	}

	res := make([]*pb.Block, len(blocks))
	for i, b := range blocks {
		res[i] = &pb.Block{
			BlockNum:   int64(b.BlockNum),
			BlockHash:  b.BlockHash,
			BlockTime:  int64(b.BlockTime),
			ParentHash: b.ParentHash,
		}
	}
	return &pb.ListLastestBlocksResponse{Blocks: res}, nil
}

func (s *EthereumServer) GetBlock(ctx context.Context, req *pb.GetBlockRequest) (*pb.GetBlockResponse, error) {
	b, err := s.svc.GetBlock(ctx, uint64(req.BlockNum))
	if err == service.ErrNotFound {
		return &pb.GetBlockResponse{}, status.Error(codes.NotFound, "block not found")
	}
	if err != nil {
		return &pb.GetBlockResponse{}, err
	}

	res := &pb.Block{
		BlockNum:     int64(b.BlockNum),
		BlockHash:    b.BlockHash,
		BlockTime:    int64(b.BlockTime),
		ParentHash:   b.ParentHash,
		Transactions: b.TxHash,
	}
	return &pb.GetBlockResponse{Block: res}, nil
}

func (s *EthereumServer) GetTransaction(ctx context.Context, req *pb.GetTransactionRequest) (*pb.GetTransactionResponse, error) {
	tx, err := s.svc.GetTransaction(ctx, req.TxHash)
	if err == service.ErrNotFound {
		return &pb.GetTransactionResponse{}, status.Error(codes.NotFound, "transaction not found")
	}
	if err != nil {
		return &pb.GetTransactionResponse{}, err
	}

	res := &pb.Transaction{
		TxHash:   tx.TxHash,
		FromAddr: tx.FromAddr,
		ToAddr:   tx.ToAddr,
		Nonce:    int64(tx.Nonce),
		Data:     tx.Data,
		Value:    tx.Value,
	}
	res.Logs = make([]*pb.Log, len(tx.Logs))
	for i, log := range tx.Logs {
		res.Logs[i] = &pb.Log{
			Index: int32(log.Index),
			Data:  log.Data,
		}
	}
	return &pb.GetTransactionResponse{Tx: res}, nil
}
