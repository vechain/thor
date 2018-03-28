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

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/pkg/errors"
	cli "gopkg.in/urfave/cli.v1"

	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

type account struct {
	Address    thor.Address
	PrivateKey *ecdsa.PrivateKey
	Balance    string
}

type cliContext struct {
	accounts     []account
	kv           *lvldb.LevelDB
	stateCreator *state.Creator
	c            *chain.Chain
	txpl         *txpool.TxPool
	pk           *packer.Packer
	ldb          *logdb.LogDB
	launchTime   uint64
}

var (
	version   string
	gitCommit string
	release   = "dev"
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
			Name:  "on-demand",
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

		solo := &cliContext{}
		solo.launchTime = uint64(time.Now().Unix())

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

		// kv, stateCreator, c, txpl, pk, ldb, err := prepare()
		err = solo.prepare()
		if err != nil {
			return errors.Wrap(err, "Preparation")
		}
		defer solo.kv.Close()

		svr := &http.Server{Handler: api.New(solo.c, solo.stateCreator, solo.txpl, solo.ldb)}
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
			solo.packing(exit, ctx.Bool("on-demand"))
		})
		goes.Go(func() {
			solo.watcher(exit)
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

func (solo *cliContext) buildGenesis() (blk *block.Block, logs []*tx.Log, err error) {
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
			return nil, nil, err
		}
		addr := crypto.PubkeyToAddress(pk.PublicKey)
		solo.accounts = append(solo.accounts, account{thor.Address(addr), pk, "10000000000000000000000"})
	}

	builder := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(solo.launchTime).
		State(func(state *state.State) error {
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes())
			state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes())

			energy := builtin.Energy.WithState(state)
			tokenSupply := &big.Int{}
			for _, a := range solo.accounts {
				b, _ := new(big.Int).SetString(a.Balance, 10)
				state.SetBalance(a.Address, b)
				energy.AddBalance(solo.launchTime, a.Address, b)
				tokenSupply.Add(tokenSupply, b)
			}

			energy.SetTokenSupply(tokenSupply)
			return nil
		}).
		Call(
			tx.NewClause(&builtin.Params.Address).
				WithData(mustEncodeInput(builtin.Params.ABI, "set", thor.KeyRewardRatio, thor.InitialRewardRatio)),
			builtin.Executor.Address).
		Call(
			tx.NewClause(&builtin.Params.Address).
				WithData(mustEncodeInput(builtin.Params.ABI, "set", thor.KeyBaseGasPrice, thor.InitialBaseGasPrice)),
			builtin.Executor.Address).
		Call(
			tx.NewClause(&builtin.Energy.Address).
				WithData(mustEncodeInput(builtin.Energy.ABI, "adjustGrowthRate", thor.InitialEnergyGrowthRate)),
			builtin.Executor.Address).
		Call(
			tx.NewClause(&builtin.Authority.Address).
				WithData(mustEncodeInput(builtin.Authority.ABI, "add", solo.accounts[0].Address, solo.accounts[0].Address, thor.BytesToHash([]byte(fmt.Sprintf("a%v", 0))))),
			builtin.Executor.Address)

	return builder.Build(solo.stateCreator)
}

func (solo *cliContext) prepare() (err error) {
	solo.kv, err = lvldb.NewMem()
	if err != nil {
		return
	}

	solo.stateCreator = state.NewCreator(solo.kv)
	solo.c = chain.New(solo.kv)

	solo.ldb, err = logdb.NewMem()
	if err != nil {
		return
	}

	b0, logs, err := solo.buildGenesis()
	if err != nil {
		return
	}

	dblogs := []*logdb.Log{}
	for index, l := range logs {
		dblog := logdb.NewLog(b0.Header(), uint32(index), thor.Hash{}, thor.Address{}, l)
		dblogs = append(dblogs, dblog)
	}
	err = solo.ldb.Insert(dblogs, nil)
	if err != nil {
		return
	}

	err = solo.c.WriteGenesis(b0)
	if err != nil {
		return
	}

	solo.txpl = txpool.New()
	solo.pk = packer.New(solo.c, solo.stateCreator, solo.accounts[0].Address, solo.accounts[0].Address)

	log.Info("Solo has been setted up successfully", "genesis block id", b0.Header().ID())
	for _, a := range solo.accounts {
		balance, _ := new(big.Int).SetString(a.Balance, 10)
		balance = balance.Div(balance, big.NewInt(1000000000000000000))
		log.Info("Builtin account info", "address", a.Address, "private key", thor.BytesToHash(crypto.FromECDSA(a.PrivateKey)), "vet balance", balance, "vethor balance", balance)
	}
	return
}

func (solo *cliContext) packing(exit <-chan interface{}, ondemand bool) {
	for {
		timeInterval := 10
		if ondemand {
			timeInterval = 1
		}
		arrive := time.After(time.Duration(timeInterval) * time.Second)
		select {
		case <-exit:
			log.Info("Killing packer service......")
			return
		case <-arrive:
			log.Trace("Try packing......")
			iter, err := solo.txpl.NewIterator(solo.c, solo.stateCreator)
			if err != nil {
				log.Error(fmt.Sprintf("%+v", err))
			}

			if iter.HasNext() || !ondemand {
				best, err := solo.c.GetBestBlock()
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}

				_, adopt, commit, err := solo.pk.Prepare(best.Header(), uint64(time.Now().Unix()))
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}

				for iter.HasNext() {
					tx := iter.Next()
					err := adopt(tx)
					if err != nil {
						log.Error(fmt.Sprintf("%+v", err))
					}
					iter.OnProcessed(tx.ID(), err)
				}

				b, receipts, err := commit(solo.accounts[0].PrivateKey)
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}
				log.Info("Packed block", "block id", b.Header().ID(), "transaction num", len(b.Transactions()), "timestamp", b.Header().Timestamp())
				log.Trace(b.String())

				err = saveBlockLogs(b, receipts, solo.ldb)
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}

				// ignore fork when solo
				_, err = solo.c.AddBlock(b, true)
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}

				err = solo.c.SetBlockReceipts(b.Header().ID(), receipts)
				if err != nil {
					log.Error(fmt.Sprintf("%+v", err))
				}
			}
		}
	}
}

func (solo *cliContext) watcher(exit <-chan interface{}) {
	ch := make(chan *tx.Transaction, 10)
	sub := solo.txpl.SubscribeNewTransaction(ch)
	defer sub.Unsubscribe()
	for {
		select {
		case tx := <-ch:
			singer, err := tx.Signer()
			if err != nil {
				singer = thor.Address{}
			}
			log.Info("New Tx", "tx id", tx.ID(), "from", singer)
			continue
		case <-exit:
			log.Info("Killing watcher service......")
			return
		}
	}
}

func saveBlockLogs(blk *block.Block, receipts tx.Receipts, ldb *logdb.LogDB) (err error) {
	logIndex := 0
	dblogs := []*logdb.Log{}

	for index, receipt := range receipts {
		for _, output := range receipt.Outputs {
			for _, l := range output.Logs {
				dblog := logdb.NewLog(blk.Header(), uint32(logIndex), blk.Transactions()[index].ID(), thor.Address{}, l)
				dblogs = append(dblogs, dblog)
				logIndex++
			}
		}
	}

	return ldb.Insert(dblogs, nil)
}

func mustEncodeInput(abi *abi.ABI, name string, args ...interface{}) []byte {
	m := abi.MethodByName(name)
	if m == nil {
		panic("no method '" + name + "'")
	}
	data, err := m.EncodeInput(args...)
	if err != nil {
		panic(err)
	}
	return data
}
