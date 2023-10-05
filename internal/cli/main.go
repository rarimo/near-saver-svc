package cli

import (
	"context"
	"gitlab.com/distributed_lab/logan/v3"

	"github.com/rarimo/near-saver-svc/internal/config"
	"github.com/rarimo/near-saver-svc/internal/services/grpc"
	"github.com/rarimo/near-saver-svc/internal/services/saver"
	voterservice "github.com/rarimo/near-saver-svc/internal/services/voter"
	rarimotypes "github.com/rarimo/rarimo-core/x/rarimocore/types"
	"github.com/rarimo/saver-grpc-lib/voter"
	"github.com/rarimo/saver-grpc-lib/voter/verifiers"
	"gitlab.com/distributed_lab/kit/kv"

	"github.com/alecthomas/kingpin"
)

func Run(args []string) bool {
	log := logan.New()

	defer func() {
		if rvr := recover(); rvr != nil {
			log.WithRecover(rvr).Error("app panicked")
		}
	}()

	cfg := config.New(kv.MustFromEnv())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log = cfg.Log()

	app := kingpin.New("near-saver-svc", "")

	runCmd := app.Command("run", "run command")

	voterCmd := runCmd.Command("voter", "run voter service")
	saverCmd := runCmd.Command("saver", "run saver service")
	saverCatchupCmd := runCmd.Command("saver-catchup", "run saver service")

	serviceCmd := runCmd.Command("service", "run service") // you can insert custom help

	cmd, err := app.Parse(args[1:])
	if err != nil {
		log.WithError(err).Error("failed to parse arguments")
		return false
	}

	switch cmd {
	case voterCmd.FullCommand():
		verifier := verifiers.NewTransferVerifier(
			voterservice.NewTransferOperator(cfg),
			cfg.Log(),
		)

		v := voter.NewVoter(cfg.ListenConf().Chain, cfg.Log(), cfg.Broadcaster(), map[rarimotypes.OpType]voter.Verifier{
			rarimotypes.OpType_TRANSFER: verifier,
		})

		// Running catchup for unvoted operations
		voter.NewCatchupper(cfg.Cosmos(), v, cfg.Log()).Run(context.TODO())
		// Running subscriber for new operations
		go voter.NewTransferSubscriber(v, cfg.Tendermint(), cfg.Cosmos(), cfg.Log(), cfg.Subscriber()).Run(ctx)

		// Running GRPC server
		err = grpc.NewSaverService(cfg.Log(), cfg.Listener(), v, cfg.Cosmos()).Run()
	case saverCmd.FullCommand():
		// Running subscriber for new transaction on bridge
		saver.New(cfg).Listen(ctx)
	case saverCatchupCmd.FullCommand():
		// Running catchup for transaction on bridge
		saver.New(cfg).Catchup(ctx)
	case serviceCmd.FullCommand():
		verifier := verifiers.NewTransferVerifier(
			voterservice.NewTransferOperator(cfg),
			cfg.Log(),
		)

		v := voter.NewVoter(cfg.ListenConf().Chain, cfg.Log(), cfg.Broadcaster(), map[rarimotypes.OpType]voter.Verifier{
			rarimotypes.OpType_TRANSFER: verifier,
		})

		// Running catchup for unvoted operations
		voter.NewCatchupper(cfg.Cosmos(), v, cfg.Log()).Run(ctx)

		// Running subscriber for new operations
		go voter.NewTransferSubscriber(v, cfg.Tendermint(), cfg.Cosmos(), cfg.Log(), cfg.Subscriber()).Run(ctx)
		// Running subscriber for new transaction on bridge
		go saver.New(cfg).Listen(ctx)

		// Running GRPC server
		err = grpc.NewSaverService(cfg.Log(), cfg.Listener(), v, cfg.Cosmos()).Run()
	default:
		log.Errorf("unknown command %s", cmd)
		return false
	}

	if err != nil {
		log.WithError(err).Error("failed to exec cmd")
		return false
	}

	return true
}
