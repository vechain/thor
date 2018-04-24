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

	"github.com/vechain/thor/api"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/transferdb"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

type account struct {
	Address    thor.Address
	PrivateKey *ecdsa.PrivateKey
	Balance    string
}

type fakeCommunicator struct {
}

type cliContext struct {
	kv           *lvldb.LevelDB
	stateCreator *state.Creator
	c            *chain.Chain
	txpl         *txpool.TxPool
	pk           *SoloPacker
	ldb          *logdb.LogDB
	tdb          *transferdb.TransferDB
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
			Name:  "api-addr",
			Value: "127.0.0.1:8669",
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
		addr, err := net.ResolveTCPAddr("tcp", ctx.String("api-addr"))
		if err != nil {
			return errors.New("Bad argument: listen address")
		}

		listener, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return errors.Wrap(err, "Creating TCP server")
		}

		log.Debug("Got addr from context", "addr", addr)

		err = solo.prepare()
		if err != nil {
			return errors.Wrap(err, "Preparation")
		}

		defer solo.kv.Close()
		defer solo.txpl.Stop()
		defer solo.ldb.Close()
		defer solo.tdb.Close()

		svr := &http.Server{Handler: api.New(solo.c, solo.stateCreator, solo.txpl, solo.ldb, solo.tdb, fakeCommunicator{})}
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

	solo.tdb, err = transferdb.NewMem()
	if err != nil {
		return
	}

	devGenesis, err := genesis.NewDevnet()
	if err != nil {
		return
	}

	b0, logs, err := devGenesis.Build(solo.stateCreator)
	if err != nil {
		return
	}

	dblogs := []*logdb.Log{}
	for index, l := range logs {
		dblog := logdb.NewLog(b0.Header(), uint32(index), thor.Bytes32{}, thor.Address{}, l)
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

	solo.pk = NewSoloPacker(solo.c, solo.stateCreator, genesis.DevAccounts()[0].Address, genesis.DevAccounts()[0].Address)

	log.Info("Solo has been setted up successfully", "genesis block id", b0.Header().ID(), "chain tag", fmt.Sprintf("0x%x", solo.c.Tag()))
	balance, _ := new(big.Int).SetString("10000000000000000000000000", 10)
	balance = balance.Div(balance, big.NewInt(1000000000000000000))
	for _, a := range genesis.DevAccounts() {
		log.Info("Builtin account info", "address", a.Address, "private key", thor.BytesToBytes32(crypto.FromECDSA(a.PrivateKey)), "vet balance", balance, "vethor balance", balance)
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
			log.Error("Excuting transaction", "error", fmt.Sprintf("%+v", err.Error()))
		}
		switch {
		case IsKnownTx(err) || IsBadTx(err):
			solo.txpl.Remove(tx.ID())
		case IsGasLimitReached(err):
			break
		case IsTxNotAdoptableNow(err):
			continue
		default:
			solo.txpl.Remove(tx.ID())
		}
	}

	b, receipts, tlogs, err := commit(genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}

	// If there is no tx packed in the on-demand mode then skip
	if solo.onDemand && len(b.Transactions()) == 0 {
		return
	}

	log.Info("Packed block", "block id", b.Header().ID(), "transaction num", len(b.Transactions()), "timestamp", b.Header().Timestamp())
	log.Debug(b.String())

	err = saveEventLogs(b, receipts, solo.ldb)
	if err != nil {
		log.Error(fmt.Sprintf("%+v", err))
	}

	err = saveTransferLogs(b, tlogs, solo.tdb)
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

func (comm fakeCommunicator) SessionCount() int {
	return 1
}

func saveEventLogs(blk *block.Block, receipts tx.Receipts, ldb *logdb.LogDB) (err error) {
	logIndex := 0
	dblogs := []*logdb.Log{}

	for index, receipt := range receipts {
		for _, output := range receipt.Outputs {
			for _, l := range output.Logs {
				txOrigin, _ := blk.Transactions()[index].Signer()
				dblog := logdb.NewLog(blk.Header(), uint32(logIndex), blk.Transactions()[index].ID(), txOrigin, l)
				dblogs = append(dblogs, dblog)
				logIndex++
			}
		}
	}

	return ldb.Insert(dblogs, nil)
}

func saveTransferLogs(blk *block.Block, transferLogs [][]tx.TransferLogs, tdb *transferdb.TransferDB) (err error) {
	logIndex := 0
	dblogs := []*transferdb.Transfer{}

	for index, txLog := range transferLogs {
		for _, clauseLog := range txLog {
			for _, log := range clauseLog {
				txOrigin, _ := blk.Transactions()[index].Signer()
				transferLog := transferdb.NewTransfer(blk.Header(), uint32(logIndex), blk.Transactions()[index].ID(), txOrigin, log)
				dblogs = append(dblogs, transferLog)
				logIndex++
			}
		}
	}

	return tdb.Insert(dblogs, nil)
}

func initLog(lvl log15.Lvl) {
	log15.Root().SetHandler(log15.LvlFilterHandler(lvl, log15.StderrHandler))
	ethLogHandler := ethlog.NewGlogHandler(ethlog.StreamHandler(os.Stderr, ethlog.TerminalFormat(true)))
	ethLogHandler.Verbosity(ethlog.LvlWarn)
	ethlog.Root().SetHandler(ethLogHandler)
}
