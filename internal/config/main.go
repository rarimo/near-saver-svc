package config

import (
	"github.com/rarimo/near-go/nearprovider"
	"github.com/rarimo/saver-grpc-lib/broadcaster"
	"github.com/rarimo/saver-grpc-lib/voter"
	"github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/distributed_lab/kit/comfig"
	"gitlab.com/distributed_lab/kit/kv"
	"google.golang.org/grpc"
)

type Config interface {
	comfig.Logger
	comfig.Listenerer
	nearprovider.Nearer
	broadcaster.Broadcasterer
	voter.Subscriberer

	Cosmos() *grpc.ClientConn
	Tendermint() *http.HTTP
	ListenConf() ListenConf
}

type config struct {
	comfig.Logger
	comfig.Listenerer
	nearprovider.Nearer
	broadcaster.Broadcasterer
	voter.Subscriberer

	getter     kv.Getter
	cosmos     comfig.Once
	tendermint comfig.Once
	lconf      comfig.Once
}

func New(getter kv.Getter) Config {
	logger := comfig.NewLogger(getter, comfig.LoggerOpts{})

	return &config{
		getter: getter,
		Logger: logger,

		Listenerer:    comfig.NewListenerer(getter),
		Nearer:        nearprovider.NewNearer(getter, logger.Log()),
		Broadcasterer: broadcaster.New(getter),
		Subscriberer:  voter.NewSubscriberer(getter),
	}
}
