package voter

import (
	"context"
	"github.com/rarimo/near-go/common"
	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
	rarimotypes "github.com/rarimo/rarimo-core/x/rarimocore/types"
	tokentypes "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"google.golang.org/grpc"
)

type NativeOperator struct {
	chain   string
	equaler *equaler
	core    *coreProvider
}

func NewNativeOperator(chain string, rarimo *grpc.ClientConn, log *logan.Entry) *NativeOperator {
	core := newCoreProvider(rarimo, log.WithField("operator", "native_operator"))
	return &NativeOperator{
		chain:   chain,
		core:    core,
		equaler: newEqualer(core),
	}
}

// Implements txParser
var _ txParser = &NativeOperator{}

func (n *NativeOperator) ParseTransaction(ctx context.Context, event *common.BridgeEvent, transfer *rarimotypes.Transfer) error {
	return n.equaler.CheckEquality(ctx, n, event, transfer)
}

func (n *NativeOperator) GetMessage(ctx context.Context, event *common.BridgeEventData) (*oracletypes.MsgCreateTransferOp, error) {
	from := tokentypes.OnChainItemIndex{
		Chain:   n.chain,
		Address: "",
		TokenID: "",
	}

	to, err := n.core.OnChainItemIndexByChain(ctx, &tokentypes.QueryGetOnChainItemByOtherRequest{Chain: n.chain, TargetChain: event.ChainTo})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get item")
	}

	msg := &oracletypes.MsgCreateTransferOp{
		Receiver: event.Receiver,
		Sender:   event.Sender,
		Amount:   *event.Amount,
		From:     from,
		To:       *to,
	}
	return appendBundleIfExists(msg, event), nil
}
