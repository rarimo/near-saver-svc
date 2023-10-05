package saver

import (
	"context"
	"github.com/rarimo/near-saver-svc/internal/services/voter"

	"github.com/rarimo/near-go/common"
	"github.com/rarimo/near-saver-svc/internal/config"
	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
	"github.com/rarimo/saver-grpc-lib/broadcaster"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type messageGetter interface {
	GetMessage(ctx context.Context, event *common.BridgeEventData) (*oracletypes.MsgCreateTransferOp, error)
}

type txProcessor struct {
	log         *logan.Entry
	contract    common.AccountID
	broadcaster broadcaster.Broadcaster
	operators   map[common.BridgeEventType]messageGetter
}

func newTxProcessor(cfg config.Config) *txProcessor {
	return &txProcessor{
		log:         cfg.Log().WithField("service", "tx_processor"),
		contract:    cfg.ListenConf().Contract,
		broadcaster: cfg.Broadcaster(),
		operators: map[common.BridgeEventType]messageGetter{
			common.NativeEventType: voter.NewNativeOperator(cfg.ListenConf().Chain, cfg.Cosmos(), cfg.Log()),
			common.FTEventType:     voter.NewFTOperator(cfg.ListenConf().Chain, cfg.Cosmos(), cfg.Log()),
			common.NFTEventType:    voter.NewNFTOperator(cfg.ListenConf().Chain, cfg.Cosmos(), cfg.Near(), cfg.Log()),
		},
	}
}

func (t *txProcessor) ProcessTx(ctx context.Context, tx, eventID string, event *common.BridgeEvent) error {
	t.log.Debug("Parsing transaction " + tx)

	operator, ok := t.operators[common.BridgeEventType(event.Event)]
	if !ok {
		t.log.WithField("event_type", event.Event).Warn("Unsupported event type")
		return nil
	}

	msg, err := operator.GetMessage(ctx, &event.Data[0])
	if err != nil {
		return errors.Wrap(err, "failed to get message")
	}

	msg.Creator = t.broadcaster.Sender()
	msg.Tx = tx
	msg.EventId = eventID

	if err = t.broadcaster.BroadcastTx(ctx, msg); err != nil {
		return errors.Wrap(err, "error broadcasting tx")
	}
	t.log.WithFields(logan.F{
		"tx":       tx,
		"eventID":  eventID,
		"receiver": msg.Receiver,
	}).Info("Successfully broadcasted transaction")

	return nil
}
