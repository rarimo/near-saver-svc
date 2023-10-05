package saver

import (
	"context"
	"github.com/rarimo/near-go/common"
	"github.com/rarimo/near-go/nearprovider"
	"github.com/rarimo/near-saver-svc/internal/config"
	"github.com/rarimo/near-saver-svc/internal/services/voter"
	"github.com/rarimo/saver-grpc-lib/voter/verifiers"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"gitlab.com/distributed_lab/running"
	"time"
)

type Saver struct {
	log       *logan.Entry
	near      nearprovider.Provider
	processor *txProcessor

	contract  common.AccountID
	batchSize uint64

	fromBlock common.BlockHeight
	toBlock   common.BlockHeight
}

var emptyBlock common.BlockHeight = 0

func New(cfg config.Config) *Saver {
	return &Saver{
		log:       cfg.Log().WithField("service", "saver"),
		near:      cfg.Near(),
		processor: newTxProcessor(cfg),
		contract:  cfg.ListenConf().Contract,
		batchSize: cfg.ListenConf().BatchSize,
		fromBlock: cfg.ListenConf().FromBlock,
	}
}

func (s *Saver) Catchup(ctx context.Context) {
	s.log.WithFields(logan.F{"batch_size": s.batchSize}).Info("Starting catchup")
	if s.fromBlock == emptyBlock {
		s.log.Info("Catchup not needed")
		return
	}

	var err error
	s.toBlock, err = s.getLastKnownBlockHeight(ctx)
	if err != nil {
		panic(errors.Wrap(err, "failed to get last known block height"))
	}

	s.log.WithFields(logan.F{"to_block": s.toBlock}).Info("Catch-upping to")

	running.UntilSuccess(ctx, s.log, "catchup", s.catchup, 1*time.Second, 5*time.Second)
}

func (s *Saver) Listen(ctx context.Context) {
	s.log.WithFields(logan.F{"batch_size": s.batchSize}).Info("Starting listening")

	var err error
	s.fromBlock, err = s.getLastKnownBlockHeight(ctx)
	if err != nil {
		panic(errors.Wrap(err, "failed to get last known block height"))
	}

	running.WithBackOff(ctx, s.log, "listener", s.listen, 1*time.Second, 1*time.Second, 5*time.Second)
}

func (s *Saver) getLastKnownBlockHeight(ctx context.Context) (common.BlockHeight, error) {
	lastBlock, err := s.near.GetLastKnownBlockHeight(ctx)
	if err != nil {
		panic(errors.Wrap(err, "failed to get last known block height"))
	}

	return *lastBlock, nil
}

func (s *Saver) handleBlocksOnce(ctx context.Context, mode string) (bool, error) {
	s.log.WithFields(logan.F{
		"mode":       mode,
		"from_block": s.fromBlock,
	}).Info("Starting iteration")

	blocks, err := s.near.ListBlocks(ctx, s.batchSize, s.fromBlock)
	if err != nil {
		return false, errors.Wrap(err, "failed to fetch blocks", logan.F{
			"start_from": s.fromBlock,
		})
	}

	if len(blocks) == 0 {
		s.log.WithFields(logan.F{
			"mode":       mode,
			"from_block": s.fromBlock,
		}).Info("No blocks to process")
		return false, nil
	}

	s.log.WithFields(logan.F{"batch_size": len(blocks)}).Debug("Got blocks batch")

	finished, err := s.processBlocks(ctx, blocks)
	if err != nil {
		return false, errors.Wrap(err, "iteration failed", logan.F{"mode": mode})
	}

	s.log.WithFields(logan.F{
		"block": blocks[len(blocks)-1],
		"mode":  mode,
	}).Infof("Iteration finished")

	return finished, nil
}

func (s *Saver) catchup(ctx context.Context) (bool, error) {
	return s.handleBlocksOnce(ctx, "catchup")
}

func (s *Saver) listen(ctx context.Context) error {
	_, err := s.handleBlocksOnce(ctx, "listening")
	return err
}

func (s *Saver) processBlocks(ctx context.Context, blocks []common.BlockHeight) (bool, error) {
	for _, block := range blocks {
		msg, err := s.near.GetMessage(ctx, block)
		if err != nil {
			return false, errors.Wrap(err, "failed to fetch message", logan.F{
				"block": block,
			})
		}

		err = s.processShards(ctx, block, msg.Shards)
		if err != nil {
			return false, errors.Wrap(err, "failed to extract events from shards")
		}

		s.fromBlock = block + 1

		if block == s.toBlock {
			// Can't be reached for listening mode, because s.toBlock is always 0
			s.log.WithField("block", block).Info("Successfully finished catchup")
			return true, nil
		}
	}

	return false, nil
}

func (s *Saver) processShards(ctx context.Context, block common.BlockHeight, shards []*common.ShardView) error {
	if len(shards) == 0 {
		s.log.WithFields(logan.F{"block": block}).Debug("No shards in block")
		return nil
	}

	for _, shard := range shards {
		if shard.Chunk == nil {
			continue
		}

		for _, transactionView := range shard.Chunk.Transactions {
			err := s.processTx(ctx, transactionView, block, shard.ShardID)
			if err != nil {
				return errors.Wrap(err, "failed to process tx")
			}
		}
	}

	return nil
}

func (s *Saver) processTx(ctx context.Context, transaction common.ShardChunkTransactionView, block common.BlockHeight, shardID common.ShardID) error {
	tx, err := s.near.GetTransaction(ctx, transaction.Transaction.Hash, transaction.Transaction.SignerID)
	if err != nil {
		return errors.Wrap(err, "failed to get transaction", logan.F{
			"block_height": block,
			"shard_id":     shardID,
			"tx_hash":      transaction.Transaction.Hash.String(),
			"sender":       transaction.Transaction.SignerID,
		})
	}

	err = s.processReceiptsOutcomes(ctx, tx.FinalExecutionOutcomeView.ReceiptsOutcome, tx.Transaction.Hash.String())
	return errors.Wrap(err, "failed to process receipts", logan.F{
		"block_height": block,
		"shard_id":     shardID,
		"tx_hash":      transaction.Transaction.Hash.String(),
		"sender":       transaction.Transaction.SignerID,
	})
}

func (s *Saver) processReceiptsOutcomes(ctx context.Context, receipts []common.ExecutionOutcomeWithIdView, txHash string) error {
	for _, receiptOutcome := range receipts {
		if receiptOutcome.Outcome.ExecutorID != s.contract {
			continue
		}

		for _, log := range receiptOutcome.Outcome.Logs {
			event := voter.GetEventFromLog(log)
			if event == nil {
				continue
			}

			err := s.processor.ProcessTx(ctx, txHash, receiptOutcome.ID.String(), event)
			if err != nil {
				// Skip the wrong operation to avoid sticking the whole process
				if errors.Cause(err) == verifiers.ErrWrongOperationContent {
					continue
				}
				return errors.Wrap(err, "failed to process tx log")
			}
		}
	}

	return nil
}
