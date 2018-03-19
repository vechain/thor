package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vechain/thor/block"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/txpool"
	cli "gopkg.in/urfave/cli.v1"
)

type account struct {
	Address    thor.Address
	PrivateKey *ecdsa.PrivateKey
	Balance    string
}

var (
	version   string
	gitCommit string
	release   = "dev"
	accounts  []account
)

func newApp() *cli.App {
	app := cli.NewApp()
	app.Version = fmt.Sprintf("%s-%s-commit%s", release, version, gitCommit)
	app.Name = "Solo"
	app.Usage = "VeChain Thor client for test & dev"
	app.Copyright = "2017 VeChain Foundation <https://vechain.org/>"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "addr",
			Value: ":7585",
			Usage: "listen address",
		},
		cli.BoolFlag{
			Name:  "ondemand",
			Usage: "create new block when there is pending transaction",
		},
		cli.IntFlag{
			Name:  "verbosity",
			Value: int(log.LvlInfo),
			Usage: "log verbosity (0-9)",
		},
		cli.StringFlag{
			Name:  "vmodule",
			Usage: "log verbosity pattern",
		},
	}
	app.Action = func(ctx *cli.Context) (err error) {
		glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(true)))
		glogger.Verbosity(log.Lvl(ctx.Int("verbosity")))
		glogger.Vmodule(ctx.String("vmodule"))
		log.Root().SetHandler(glogger)

		goes := &co.Goes{}
		defer goes.Wait()

		// check addr and create tcp listener
		addr, err := net.ResolveTCPAddr("tcp", ctx.String("addr"))
		if err != nil {
			return errors.New("Bad argument: listen address")
		}

		listener, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return errors.Wrap(err, "Creating TCP server")
		}

		log.Debug("Got addr from context", "addr", addr)

		kv, stateCreator, c, txpl, pk, ldb, err := prepare()
		defer kv.Close()
		if err != nil {
			return errors.Wrap(err, "Preparation")
		}

		svr := &http.Server{Handler: api.NewHTTPHandler(c, stateCreator, txpl, ldb)}
		defer svr.Shutdown(context.Background())
		defer log.Info("Killing restful service......")

		// setting up channels
		quit := make(chan os.Signal)
		signal.Notify(quit,
			syscall.SIGINT, syscall.SIGTERM,
			syscall.SIGHUP, syscall.SIGKILL,
			syscall.SIGUSR1, syscall.SIGUSR2)
		exit := make(chan interface{})

		// run services
		goes.Go(func() {
			// ignore error from http server
			_ = svr.Serve(listener)
		})
		goes.Go(func() {
			packing(exit, pk, txpl, stateCreator, c, ctx.Bool("ondemand"))
		})

		select {
		case <-quit:
			log.Info("Got interrupt, cleaning services......")
			close(exit)
		}
		return
	}
	return app
}

func main() {
	if err := newApp().Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildGenesis(stateCreator *state.Creator) (blk *block.Block, err error) {
	launchTime := uint64(time.Now().UnixNano())

	privKeys := []string{
		"dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65",
		"321d6443bc6177273b5abf54210fe806d451d6b7973bccc2384ef78bbcd0bf51",
		"2d7c882bad2a01105e36dda3646693bc1aaaa45b0ed63fb0ce23c060294f3af2",
		"593537225b037191d322c3b1df585fb1e5100811b71a6f7fc7e29cca1333483e",
		"ca7b25fc980c759df5f3ce17a3d881d6e19a38e651fc4315fc08917edab41058",
		"88d2d80b12b92feaa0da6d62309463d20408157723f2d7e799b6a74ead9a673b",
		"fbb9e7ba5fe9969a71c6599052237b91adeb1e5fc0c96727b66e56ff5d02f9d0",
		"547fb081e73dc2e22b4aae5c60e2970b008ac4fc3073aebc27d41ace9c4f53e9",
		"c8c53657e41a8d669349fc287f57457bd746cb1fcfc38cf94d235deb2cfca81b",
		"87e0eba9c86c494d98353800571089f316740b0cb84c9a7cdf2fe5c9997c7966",
	}
	for _, str := range privKeys {
		pk, err := crypto.HexToECDSA(str)
		if err != nil {
			return nil, err
		}
		addr := crypto.PubkeyToAddress(pk.PublicKey)
		accounts = append(accounts, account{thor.Address(addr), pk, "10000000000000000000000"})
	}

	builder := new(genesis.Builder).
		ChainTag(1).
		GasLimit(thor.InitialGasLimit).
		Timestamp(launchTime).
		State(func(state *state.State) error {
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
			state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())

			builtin.Params.Set(state, thor.KeyRewardRatio, thor.InitialRewardRatio)
			builtin.Params.Set(state, thor.KeyBaseGasPrice, thor.InitialBaseGasPrice)
			builtin.Energy.AdjustGrowthRate(state, launchTime, thor.InitialEnergyGrowthRate)
			builtin.Authority.Add(state, accounts[0].Address, thor.Hash{})

			for _, a := range accounts {
				b, _ := new(big.Int).SetString(a.Balance, 10)
				state.SetBalance(a.Address, b)
				builtin.Energy.AddBalance(state, launchTime, a.Address, b)
			}
			return nil
		})

	return builder.Build(stateCreator)
}

func prepare() (kv *lvldb.LevelDB, stateCreator *state.Creator, c *chain.Chain, txpl *txpool.TxPool, pk *packer.Packer, ldb *logdb.LogDB, err error) {
	kv, err = lvldb.NewMem()
	if err != nil {
		return
	}

	stateCreator = state.NewCreator(kv)
	c = chain.New(kv)

	b0, err := buildGenesis(stateCreator)
	if err != nil {
		return
	}

	ldb, err = logdb.NewMem()
	if err != nil {
		return
	}

	err = c.WriteGenesis(b0)
	if err != nil {
		return
	}

	txpl = txpool.New()
	pk = packer.New(c, stateCreator, accounts[0].Address, accounts[0].Address)

	log.Info("Solo has been setted up successfully", "genesis block id", b0.Header().ID())
	for _, a := range accounts {
		balance, _ := new(big.Int).SetString(a.Balance, 10)
		balance = balance.Div(balance, big.NewInt(1000000000000000000))
		log.Info("Builtin account info", "address", a.Address, "private key", thor.BytesToHash(crypto.FromECDSA(a.PrivateKey)), "vet balance", balance, "vethor balance", balance)
	}
	return
}

func packing(exit <-chan interface{}, pk *packer.Packer, txpl *txpool.TxPool, stateCreator *state.Creator, c *chain.Chain, ondemand bool) {
	for {
		arrive := time.After(10 * time.Second)
		select {
		case <-exit:
			log.Info("Killing packer service......")
			return
		case <-arrive:
			iter, err := txpl.NewIterator(c, stateCreator)
			if err != nil {
				log.Error(fmt.Sprintf("%+v", err))
			}

			if iter.HasNext() || !ondemand {
				best, err := c.GetBestBlock()
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}

				_, adopt, commit, err := pk.Prepare(best.Header(), uint64(time.Now().Unix()))
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}

				for iter.HasNext() {
					tx := iter.Next()
					adopt(tx)
				}

				b, receipts, err := commit(accounts[0].PrivateKey)
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}
				log.Info("Packed block", "block id", b.Header().ID(), "transaction num", len(b.Transactions()), "timestamp", b.Header().Timestamp())

				// ignore fork when solo
				_, err = c.AddBlock(b, true)
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}

				err = c.SetBlockReceipts(b.Header().ID(), receipts)
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}
			}
		}
	}
}
