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
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/inconshreveable/log15"
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
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	"github.com/vechain/thor/vm/evm"
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
	pk           *SoloPacker
	ldb          *logdb.LogDB
	launchTime   uint64
	onDemand     bool
}

var (
	version   string
	gitCommit string
	release   = "dev"
	log       = log15.New()
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
			Value: int(log15.LvlInfo),
			Usage: "log verbosity (0-9)",
		},
		cli.StringFlag{
			Name:  "vmodule",
			Usage: "log verbosity pattern",
		},
	}
	app.Action = func(ctx *cli.Context) (err error) {
		initLog(log15.Lvl(ctx.Int("verbosity")))

		solo := &cliContext{
			onDemand:   ctx.Bool("on-demand"),
			launchTime: uint64(time.Now().Unix()),
		}

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
		done := make(chan interface{})

		// run services
		goes.Go(func() {
			// ignore error from http server
			_ = svr.Serve(listener)
		})
		goes.Go(func() {
			solo.interval(done)
		})
		goes.Go(func() {
			solo.watcher(done)
		})

		select {
		case <-quit:
			log.Info("Got interrupt, cleaning services......")
			close(done)
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
			// alloc precompiled contracts
			for addr := range evm.PrecompiledContractsByzantium {
				state.SetBalance(thor.Address(addr), big.NewInt(1))
			}
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
			energy.InitializeTokenSupply(solo.launchTime, tokenSupply)
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

	solo.c, err = chain.New(solo.kv, b0)
	if err != nil {
		return
	}

	solo.txpl = txpool.New(solo.c, solo.stateCreator)
	defer solo.txpl.Stop()

	solo.pk = NewSoloPacker(solo.c, solo.stateCreator, solo.accounts[0].Address, solo.accounts[0].Address)

	log.Info("Solo has been setted up successfully", "genesis block id", b0.Header().ID())
	for _, a := range solo.accounts {
		balance, _ := new(big.Int).SetString(a.Balance, 10)
		balance = balance.Div(balance, big.NewInt(1000000000000000000))
		log.Info("Builtin account info", "address", a.Address, "private key", thor.BytesToHash(crypto.FromECDSA(a.PrivateKey)), "vet balance", balance, "vethor balance", balance)
	}
	return
}

func (solo *cliContext) interval(done <-chan interface{}) {
	if solo.onDemand {
		return
	}
	for {
		timeInterval := 10
		arrive := time.After(time.Duration(timeInterval) * time.Second)
		select {
		case <-done:
			log.Info("Killing interval packing service......")
			return
		case <-arrive:
			solo.packing()
		}
	}
}

func (solo *cliContext) packing() {
	log.Debug("Try packing......")

	best, err := solo.c.GetBestBlock()
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}

	adopt, commit, err := solo.pk.Prepare(best.Header(), uint64(time.Now().Unix()))
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}

	pendingTxs := solo.txpl.Pending()
	for _, tx := range pendingTxs {
		err := adopt(tx)
		if err != nil {
			log.Error("Excuting transaction", "error", fmt.Sprintf("%+v", err))
		}
		solo.txpl.OnProcessed(tx.ID(), err)
	}

	b, receipts, err := commit(solo.accounts[0].PrivateKey)
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}

	// If there is no tx packed in the on-demand mode then skip
	if solo.onDemand && len(b.Transactions()) == 0 {
		return
	}

	log.Info("Packed block", "block id", b.Header().ID(), "transaction num", len(b.Transactions()), "timestamp", b.Header().Timestamp())
	log.Debug(b.String())

	err = saveBlockLogs(b, receipts, solo.ldb)
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}

	// ignore fork when solo
	_, err = solo.c.AddBlock(b, receipts, true)
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}
}

func (solo *cliContext) watcher(done <-chan interface{}) {
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
			if solo.onDemand {
				solo.packing()
			}
			continue
		case <-done:
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

func initLog(lvl log15.Lvl) {
	log15.Root().SetHandler(log15.LvlFilterHandler(lvl, log15.StderrHandler))
	// set go-ethereum log lvl to Warn
	ethLogHandler := ethlog.NewGlogHandler(ethlog.StreamHandler(os.Stderr, ethlog.TerminalFormat(true)))
	ethLogHandler.Verbosity(ethlog.LvlWarn)
	ethlog.Root().SetHandler(ethLogHandler)
}
