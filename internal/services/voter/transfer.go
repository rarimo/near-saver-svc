package voter

import (
	"context"
	"github.com/rarimo/near-go/common"
	"github.com/rarimo/near-go/nearprovider"
	"github.com/rarimo/near-saver-svc/internal/config"
	rarimotypes "github.com/rarimo/rarimo-core/x/rarimocore/types"
	"github.com/rarimo/saver-grpc-lib/voter/verifiers"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type txParser interface {
	ParseTransaction(ctx context.Context, event *common.BridgeDepositedEvent, transfer *rarimotypes.Transfer) error
}

type TransferOperator struct {
	near      nearprovider.Provider
	contract  common.AccountID
	chain     string
	txParsers map[common.BridgeEventType]txParser
}

func NewTransferOperator(cfg config.Config) *TransferOperator {
	return &TransferOperator{
		near:     cfg.Near(),
		contract: cfg.ListenConf().Contract,
		chain:    cfg.ListenConf().Chain,
		txParsers: map[common.BridgeEventType]txParser{
			common.EventTypeNFTDeposited:    NewNFTOperator(cfg.ListenConf().Chain, cfg.Cosmos(), cfg.Near(), cfg.Log()),
			common.EventTypeFTDeposited:     NewFTOperator(cfg.ListenConf().Chain, cfg.Cosmos(), cfg.Log()),
			common.EventTypeNativeDeposited: NewNativeOperator(cfg.ListenConf().Chain, cfg.Cosmos(), cfg.Log()),
		},
	}
}

// Implements verifiers.ITransferOperator
var _ verifiers.TransferOperator = &TransferOperator{}

func (t *TransferOperator) VerifyTransfer(ctx context.Context, tx, eventId string, transfer *rarimotypes.Transfer) error {
	if transfer.From.Chain != t.chain {
		return verifiers.ErrUnsupportedNetwork
	}

	txHash, err := common.NewCryptoHashFromBase58(tx)
	if err != nil {
		return errors.Wrap(err, "failed to parse tx hash", logan.F{"tx": tx})
	}

	transaction, err := t.near.GetTransaction(ctx, txHash, transfer.Sender)
	if err != nil {
		return errors.Wrap(err, "failed to get transaction", logan.F{"tx": tx, "sender": transfer.Sender})
	}

	event, err := extractEvent(transaction, t.contract, eventId)
	if err != nil {
		if errors.Cause(err) == ErrEventNotFound {
			return verifiers.ErrWrongOperationContent
		}
		return errors.Wrap(err, "failed to extract event", logan.F{"raw": tx, "sender": transfer.Sender, "event_id": eventId})
	}

	if operator, ok := t.txParsers[common.BridgeEventType(event.Event)]; ok {
		return operator.ParseTransaction(ctx, event, transfer)
	}

	return nil
}
