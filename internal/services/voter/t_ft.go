package voter

import (
	"context"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/rarimo/near-go/common"
	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
	rarimotypes "github.com/rarimo/rarimo-core/x/rarimocore/types"
	tokentypes "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"google.golang.org/grpc"
)

type FTOperator struct {
	chain   string
	equaler *equaler
	core    *coreProvider
}

func NewFTOperator(chain string, rarimo *grpc.ClientConn, log *logan.Entry) *FTOperator {
	core := newCoreProvider(rarimo, log.WithField("operator", "ft_operator"))
	return &FTOperator{
		chain:   chain,
		core:    core,
		equaler: newEqualer(core),
	}
}

// Implements txParser
var _ txParser = &FTOperator{}

func (f *FTOperator) ParseTransaction(ctx context.Context, event *common.BridgeEvent, transfer *rarimotypes.Transfer) error {
	return f.equaler.CheckEquality(ctx, f, event, transfer)
}

func (f *FTOperator) GetMessage(ctx context.Context, event *common.BridgeEventData) (*oracletypes.MsgCreateTransferOp, error) {
	from := tokentypes.OnChainItemIndex{
		Chain:   f.chain,
		Address: hexutil.Encode([]byte(*event.Token)),
		TokenID: "",
	}

	to, err := f.core.OnChainItemIndexByChain(ctx, &tokentypes.QueryGetOnChainItemByOtherRequest{Chain: f.chain, Address: from.Address, TargetChain: event.ChainTo})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get item")
	}

	msg := &oracletypes.MsgCreateTransferOp{
		Sender:   event.Sender,
		Receiver: event.Receiver,
		Amount:   *event.Amount,
		From:     from,
		To:       *to,
	}

	return appendBundleIfExists(msg, event), nil
}
