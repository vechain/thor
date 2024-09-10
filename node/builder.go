package node

import (
	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type TxAndRcpt struct {
	Transaction *tx.Transaction
	ReceiptFunc func(tx.Receipts)
}

func RegisterAccountsAPI(
	gasLimit uint64,
	forkConfig thor.ForkConfig,
	bftFunc func(repo *chain.Repository) bft.Committer,
) func(repo *chain.Repository, stater *state.Stater, router *mux.Router) {
	return func(repo *chain.Repository, stater *state.Stater, router *mux.Router) {
		accounts.New(repo, stater, gasLimit, forkConfig, bftFunc(repo)).Mount(router, "/accounts")
	}
}

type Builder struct {
	dbFunc      func() *muxdb.MuxDB
	genesisFunc func() *genesis.Genesis
	engineFunc  func(repo *chain.Repository) bft.Committer
	router      *mux.Router
	chain       *Chain
}

func (b *Builder) WithDB(memFunc func() *muxdb.MuxDB) *Builder {
	b.dbFunc = memFunc
	return b
}

func (b *Builder) WithGenesis(genesisFunc func() *genesis.Genesis) *Builder {
	b.genesisFunc = genesisFunc
	return b
}

func (b *Builder) WithBFTEngine(engineFunc func(repo *chain.Repository) bft.Committer) *Builder {
	b.engineFunc = engineFunc
	return b
}

func (b *Builder) WithAPIs(apis ...utils.APIServer) *Builder {
	b.router = mux.NewRouter()

	for _, api := range apis {
		api.MountDefaultPath(b.router)
	}

	return b
}

func (b *Builder) Build() (*Node, error) {
	return &Node{
		chain:  b.chain,
		router: b.router,
	}, nil
}

func (b *Builder) WithChain(chain *Chain) *Builder {
	b.chain = chain
	return b
}
