package node

import (
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
)

type ChainBuilder struct {
	dbFunc      func() *muxdb.MuxDB
	genesisFunc func() *genesis.Genesis
	engineFunc  func(repo *chain.Repository) bft.Committer
}

func (b *ChainBuilder) WithDB(db func() *muxdb.MuxDB) *ChainBuilder {
	b.dbFunc = db
	return b
}

func (b *ChainBuilder) WithGenesis(genesisFunc func() *genesis.Genesis) *ChainBuilder {
	b.genesisFunc = genesisFunc
	return b
}

func (b *ChainBuilder) WithBFTEngine(engineFunc func(repo *chain.Repository) bft.Committer) *ChainBuilder {
	b.engineFunc = engineFunc
	return b
}

func (b *ChainBuilder) Build() (*Chain, error) {
	db := b.dbFunc()
	stater := state.NewStater(db)
	gene := b.genesisFunc()
	geneBlk, _, _, err := gene.Build(stater)
	if err != nil {
		return nil, err
	}

	repo, err := chain.NewRepository(db, geneBlk)
	if err != nil {
		return nil, err
	}

	return &Chain{
		db:           db,
		genesis:      gene,
		genesisBlock: geneBlk,
		repo:         repo,
		stater:       stater,
		engine:       b.engineFunc(repo),
	}, nil
}

func NewIntegrationTestChain() (*Chain, error) {
	return new(ChainBuilder).
		WithDB(muxdb.NewMem).
		WithGenesis(genesis.NewDevnet).
		WithBFTEngine(solo.NewBFTEngine).
		Build()
}
