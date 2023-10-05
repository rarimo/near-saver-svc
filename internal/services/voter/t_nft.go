package voter

import (
	"encoding/base64"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/rarimo/near-go/common"
	"github.com/rarimo/near-go/nearprovider"
	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
	rarimotypes "github.com/rarimo/rarimo-core/x/rarimocore/types"
	tokentypes "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"github.com/rarimo/saver-grpc-lib/voter/verifiers"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type NFTOperator struct {
	chain   string
	equaler *equaler
	near    nearprovider.Provider
	core    *coreProvider
}

func NewNFTOperator(chain string, rarimo *grpc.ClientConn, near nearprovider.Provider, log *logan.Entry) *NFTOperator {
	core := newCoreProvider(rarimo, log.WithField("operator", "nft_operator"))
	return &NFTOperator{
		chain:   chain,
		equaler: newEqualer(core),
		near:    near,
		core:    core,
	}
}

// Implements txParser
var _ txParser = &NFTOperator{}

func (n *NFTOperator) ParseTransaction(ctx context.Context, event *common.BridgeEvent, transfer *rarimotypes.Transfer) error {
	return n.equaler.CheckEquality(ctx, n, event, transfer)
}

func (n *NFTOperator) GetMessage(ctx context.Context, event *common.BridgeEventData) (*oracletypes.MsgCreateTransferOp, error) {
	addressFrom := hexutil.Encode([]byte(*event.Token))
	tokenIdFrom := *event.TokenID

	nativeCollectionData, err := n.getNativeData(ctx, n.chain, addressFrom)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get native collection data")
	}

	// if native chain is Near we should take hex from token address string, otherwise token id is already in hex format
	if nativeCollectionData.TokenType == tokentypes.Type_NEAR_NFT {
		tokenIdFrom = hexutil.Encode([]byte(*event.TokenID))
	}

	from := tokentypes.OnChainItemIndex{
		Chain:   n.chain,
		Address: addressFrom,
		TokenID: tokenIdFrom,
	}

	to, err := n.getTargetOnChainItem(ctx, nativeCollectionData, &from, event.ChainTo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get to chain item index")
	}

	meta, isGenerated, err := n.getItemMeta(ctx, &from)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get item meta")
	}

	if to.TokenID == "" { // tokenId is empty only in case of Solana target chain, so should be generated here
		to.TokenID, err = n.core.TokenPDAId(ctx, meta.Seed)
		if err != nil {
			return nil, errors.Wrap(err, "error getting token pda id")
		}
	}

	msg := &oracletypes.MsgCreateTransferOp{
		Receiver: event.Receiver,
		Sender:   event.Sender,
		Amount:   "1",
		From:     from,
		To:       *to,
	}

	if isGenerated {
		msg.Meta = meta
	}

	return appendBundleIfExists(msg, event), nil
}

// getTargetOnChainItem generates target OnChainItem based on current item information and its native mint information
// If target exists => use its data
// If target chain is Solana => target id will be derived using generated seed
// If native chain is EVM => target id will be equal to the EVM
// If native chain is Near => target id will be equal to the current one
func (n *NFTOperator) getTargetOnChainItem(ctx context.Context, nativeCollectionData *tokentypes.CollectionData, from *tokentypes.OnChainItemIndex, chainTo string) (*tokentypes.OnChainItemIndex, error) {
	// 1. checking corner cases (from == to)
	if from.Chain == chainTo {
		return from, nil
	}

	// 2. trying to check the existence of target OnChainItem
	to, err := n.tryGetOnChainItem(ctx, from, chainTo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get on chain item")
	}

	// 3. if exists - return it
	if to != nil {
		return to, nil
	}

	// 4. getting target data (should exist)
	targetData, err := n.getTargetData(ctx, from, chainTo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get target data index")
	}

	// 5. If its equal to the current chain
	if nativeCollectionData.Index.Chain == from.Chain {
		return &tokentypes.OnChainItemIndex{
			Chain:   targetData.Index.Chain,
			Address: targetData.Index.Address,
			TokenID: from.TokenID,
		}, nil
	}

	// 6. getting native OnChainItem (should exist)
	native, err := n.tryGetOnChainItem(ctx, from, nativeCollectionData.Index.Chain)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get native on chain item")
	}

	if native == nil {
		return nil, verifiers.ErrWrongOperationContent
	}

	// 7. if target chain is solana - return chain item index without token id to populate it from metadata with program address
	if targetData.TokenType == tokentypes.Type_METAPLEX_NFT {
		return &tokentypes.OnChainItemIndex{
			Chain:   targetData.Index.Chain,
			Address: targetData.Index.Address,
		}, nil
	}

	// 8. then target token id is equal to the native token id (in case of Near chain it already store in hex format)
	return &tokentypes.OnChainItemIndex{
		Chain:   targetData.Index.Chain,
		Address: targetData.Index.Address,
		TokenID: native.TokenID,
	}, nil
}

func (n *NFTOperator) getNativeData(ctx context.Context, chain, address string) (*tokentypes.CollectionData, error) {
	collection, err := n.core.CollectionByCollectionData(ctx, &tokentypes.QueryGetCollectionByCollectionDataRequest{
		Chain:   chain,
		Address: address,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get collection by collection data")
	}

	nativeCollectionData, err := n.core.NativeCollectionData(ctx, &tokentypes.QueryGetNativeCollectionDataRequest{Collection: collection.Index})
	if err != nil {
		return nil, errors.Wrap(err, "error fetching native collection data")
	}

	return nativeCollectionData, nil
}

func (n *NFTOperator) getTargetData(ctx context.Context, from *tokentypes.OnChainItemIndex, chainTo string) (*tokentypes.CollectionData, error) {
	collection, err := n.core.CollectionByCollectionData(ctx, &tokentypes.QueryGetCollectionByCollectionDataRequest{
		Chain:   from.Chain,
		Address: from.Address,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get collection by collection data")
	}

	for _, index := range collection.Data {
		if index.Chain == chainTo {
			return n.core.CollectionData(ctx, &tokentypes.QueryGetCollectionDataRequest{
				Chain:   index.Chain,
				Address: index.Address,
			})
		}
	}

	return nil, verifiers.ErrWrongOperationContent
}

func (n *NFTOperator) tryGetOnChainItem(ctx context.Context, from *tokentypes.OnChainItemIndex, chainTo string) (*tokentypes.OnChainItemIndex, error) {
	toOnChainItemIndex, err := n.core.OnChainItemByOther(ctx, &tokentypes.QueryGetOnChainItemByOtherRequest{
		Chain:       from.Chain,
		Address:     from.Address,
		TokenID:     from.TokenID,
		TargetChain: chainTo,
	})
	if err != nil {
		res, ok := status.FromError(errors.Cause(err))
		if ok && res.Code() == codes.NotFound {
			return nil, nil
		}

		return nil, errors.Wrap(err, "failed to get on chain item by other")
	}

	return toOnChainItemIndex, nil
}

func (n *NFTOperator) getItemMeta(ctx context.Context, from *tokentypes.OnChainItemIndex) (*tokentypes.ItemMetadata, bool, error) {
	item, err := n.core.ItemByOnChainItem(ctx, &tokentypes.QueryGetItemByOnChainItemRequest{
		Chain:   from.Chain,
		Address: from.Address,
		TokenID: from.TokenID,
	})

	if err != nil {
		metadata, err := n.near.GetNFTMetadata(ctx, string(hexutil.MustDecode(from.Address)), string(hexutil.MustDecode(from.TokenID)))
		if err != nil {
			return nil, false, errors.Wrap(err, "error fetching metadata from chain")
		}

		seed, _, err := n.core.GenerateTokenSeed(ctx)
		if err != nil {
			return nil, false, err
		}

		return &tokentypes.ItemMetadata{
			ImageUri:  metadata.Media,
			ImageHash: base64.StdEncoding.EncodeToString(metadata.MediaHash),
			Seed:      seed,
			Uri:       metadata.Reference,
		}, true, nil
	}

	return &item.Meta, false, nil
}
