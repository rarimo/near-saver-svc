package config

import (
	"github.com/rarimo/near-go/common"
	"gitlab.com/distributed_lab/figure"
	"gitlab.com/distributed_lab/kit/kv"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type ListenConf struct {
	Contract  common.AccountID   `fig:"contract,required"`
	FromBlock common.BlockHeight `fig:"from_block,required"`
	BatchSize uint64             `fig:"batch_size,required"`
	Chain     string             `fig:"chain"`
}

func (c *config) ListenConf() ListenConf {
	return c.lconf.Do(func() interface{} {
		config := ListenConf{}

		if err := figure.Out(&config).
			With(figure.BaseHooks, figure.Hooks{
				"common.BlockHeight": figure.BaseHooks["uint64"],
				"common.AccountID":   figure.BaseHooks["string"],
			}).
			From(kv.MustGetStringMap(c.getter, "listen")).
			Please(); err != nil {
			panic(errors.Wrap(err, "failed to figure out"))
		}

		return config
	}).(ListenConf)
}
