package voter

import (
	"context"
	oracletypes "github.com/rarimo/rarimo-core/x/oraclemanager/types"
	rarimotypes "github.com/rarimo/rarimo-core/x/rarimocore/types"
	tokentypes "github.com/rarimo/rarimo-core/x/tokenmanager/types"
	"github.com/rarimo/saver-grpc-lib/voter/verifiers"
	"gitlab.com/distributed_lab/logan/v3"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"google.golang.org/grpc"
)

type coreProvider struct {
	log           *logan.Entry
	tokenQuerier  tokentypes.QueryClient
	oracleQuerier oracletypes.QueryClient
}

func newCoreProvider(rarimo *grpc.ClientConn, log *logan.Entry) *coreProvider {
	return &coreProvider{
		log:           log,
		tokenQuerier:  tokentypes.NewQueryClient(rarimo),
		oracleQuerier: oracletypes.NewQueryClient(rarimo),
	}
}

func (p *coreProvider) ItemByOnChainItem(ctx context.Context, req *tokentypes.QueryGetItemByOnChainItemRequest) (*tokentypes.Item, error) {
	resp, err := p.tokenQuerier.ItemByOnChainItem(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get item")
	}

	return &resp.Item, nil
}

func (p *coreProvider) OnChainItemByOther(ctx context.Context, req *tokentypes.QueryGetOnChainItemByOtherRequest) (*tokentypes.OnChainItemIndex, error) {
	resp, err := p.tokenQuerier.OnChainItemByOther(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get on chain item by other")
	}

	return resp.Item.Index, nil
}

func (p *coreProvider) OnChainItemIndexByChain(ctx context.Context, req *tokentypes.QueryGetOnChainItemByOtherRequest) (*tokentypes.OnChainItemIndex, error) {
	item, err := p.tokenQuerier.OnChainItemByOther(ctx, req)
	if err != nil {
		p.log.WithError(err).Error("error fetching on chain item")
		return nil, verifiers.ErrWrongOperationContent
	}

	return item.Item.Index, nil
}

func (p *coreProvider) CollectionByCollectionData(ctx context.Context, req *tokentypes.QueryGetCollectionByCollectionDataRequest) (*tokentypes.Collection, error) {
	resp, err := p.tokenQuerier.CollectionByCollectionData(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching collection")
	}

	return &resp.Collection, nil
}

func (p *coreProvider) CollectionData(ctx context.Context, req *tokentypes.QueryGetCollectionDataRequest) (*tokentypes.CollectionData, error) {
	resp, err := p.tokenQuerier.CollectionData(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching collection data")
	}

	return &resp.Data, nil
}

func (p *coreProvider) NativeCollectionData(ctx context.Context, req *tokentypes.QueryGetNativeCollectionDataRequest) (*tokentypes.CollectionData, error) {
	resp, err := p.tokenQuerier.NativeCollectionData(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching native collection data")
	}

	return &resp.Data, nil
}

func (p *coreProvider) NetworkParams(ctx context.Context) ([]*tokentypes.Network, error) {
	resp, err := p.tokenQuerier.Params(ctx, &tokentypes.QueryParamsRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "error fetching params")
	}

	return resp.Params.Networks, nil
}

func (p *coreProvider) SolanaNetworkParams(ctx context.Context) (*tokentypes.Network, error) {
	// TODO: remove this method after adding support for multiple solana networks
	networks, err := p.NetworkParams(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network params")
	}

	for _, network := range networks {
		if network.Type == tokentypes.NetworkType_Solana {
			return network, nil
		}
	}

	return nil, errors.New("no solana network params found")
}

func (p *coreProvider) GenerateTokenSeed(ctx context.Context) (string, string, error) {
	network, err := p.SolanaNetworkParams(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get network params")
	}

	bridgeparams := network.GetBridgeParams()
	if bridgeparams == nil {
		return "", "", errors.New("bridge params not found")
	}

	seed, tokenID := verifiers.MustGenerateTokenSeed(bridgeparams.Contract)
	return seed, tokenID, nil
}

func (p *coreProvider) TokenPDAId(ctx context.Context, seed string) (string, error) {
	network, err := p.SolanaNetworkParams(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get network params")
	}

	bridgeparams := network.GetBridgeParams()
	if bridgeparams == nil {
		return "", errors.New("bridge params not found")
	}

	return verifiers.MustGetPDA(bridgeparams.Contract, seed), nil
}

func (p *coreProvider) IsSeedExist(ctx context.Context, seed string) bool {
	_, err := p.tokenQuerier.Seed(ctx, &tokentypes.QueryGetSeedRequest{Seed: seed})
	return err != nil
}

func (p *coreProvider) Transfer(ctx context.Context, req *oracletypes.QueryGetTransferRequest) (*rarimotypes.Transfer, error) {
	transferResp, err := p.oracleQuerier.Transfer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "error querying transfer from core")
	}

	return &transferResp.Transfer, nil
}
