package voter

import (
	"context"
	"github.com/rarimo/near-go/common"

	"github.com/gogo/protobuf/proto"
	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
	rarimotypes "github.com/rarimo/rarimo-core/x/rarimocore/types"
	"github.com/rarimo/saver-grpc-lib/voter/verifiers"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type messageGetter interface {
	GetMessage(ctx context.Context, event *common.BridgeEventData) (*oracletypes.MsgCreateTransferOp, error)
}

type equaler struct {
	core *coreProvider
}

func newEqualer(core *coreProvider) *equaler {
	return &equaler{
		core: core,
	}
}

func (e *equaler) CheckEquality(ctx context.Context, operator messageGetter, event *common.BridgeEvent, transfer *rarimotypes.Transfer) error {
	eventData := event.Data[0]
	msg, err := operator.GetMessage(ctx, &eventData)
	if err != nil {
		return errors.Wrap(err, "error getting message")
	}

	msg.Tx = transfer.Tx
	msg.EventId = transfer.EventId

	if msg.Meta == nil && transfer.Meta != nil || msg.Meta != nil && transfer.Meta == nil {
		return verifiers.ErrWrongOperationContent
	}

	// if its NFT event, we need to verify seed
	if common.BridgeEventType(event.Event) == common.NFTEventType && transfer.Meta != nil {
		msg.Meta.Seed = transfer.Meta.Seed
		msg.To.TokenID = transfer.To.TokenID

		network, err := e.core.SolanaNetworkParams(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get network params")
		}

		bridgeparams := network.GetBridgeParams()
		if bridgeparams == nil {
			return errors.New("bridge params not found")
		}

		if ok := verifiers.MustVerifyTokenSeed(bridgeparams.Contract, msg.Meta.Seed); !ok {
			return verifiers.ErrWrongOperationContent
		}

		if pda := verifiers.MustGetPDA(bridgeparams.Contract, msg.Meta.Seed); pda != msg.To.TokenID {
			return verifiers.ErrWrongOperationContent
		}

		if e.core.IsSeedExist(ctx, msg.Meta.Seed) {
			return verifiers.ErrWrongOperationContent
		}
	}

	sourceTransfer, err := e.core.Transfer(ctx, &oracletypes.QueryGetTransferRequest{Msg: *msg})
	if err != nil {
		return errors.Wrap(err, "error querying transfer from core")
	}

	if !proto.Equal(sourceTransfer, transfer) {
		return verifiers.ErrWrongOperationContent
	}

	return nil
}
