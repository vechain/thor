package consensus

import (
	"crypto/ecdsa"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
)

func TestConsensus(t *testing.T) {
	assert := assert.New(t)
	tc := newTestConsensus(t)

	triggerErrInterval(tc, assert)

}

func triggerErrInterval(tc *testConsensus, assert *assert.Assertions) {
	build := tc.originalBuilder()
	blk := tc.sign(build.Timestamp(tc.original.Header().Timestamp() + 1).Build())
	err := tc.consent(blk)
	expect := consensusError(
		fmt.Sprintf(
			"block interval not rounded: parent %v, current %v",
			tc.parent.Header().Timestamp(),
			blk.Header().Timestamp(),
		),
	)
	assert.Equal(err, expect)
}

type testConsensus struct {
	t        *testing.T
	con      *Consensus
	time     uint64
	pk       *ecdsa.PrivateKey
	parent   *block.Block
	original *block.Block
}

func newTestConsensus(t *testing.T) *testConsensus {
	db, err := lvldb.NewMem()
	if err != nil {
		t.Fatal(err)
	}

	gen, err := genesis.NewDevnet()
	if err != nil {
		t.Fatal(err)
	}

	stateCreator := state.NewCreator(db)
	parent, _, err := gen.Build(stateCreator)
	if err != nil {
		t.Fatal(err)
	}

	c, err := chain.New(db, parent)
	if err != nil {
		t.Fatal(err)
	}

	proposer := genesis.DevAccounts()[rand.Intn(len(genesis.DevAccounts()))]
	p := packer.New(c, stateCreator, proposer.Address, proposer.Address)
	flow, err := p.Schedule(parent.Header(), uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}

	dv := flow.When() - uint64(time.Now().Unix())
	fmt.Println("wait for pack:", dv, "s")
	timer := time.NewTimer(time.Duration(dv) * time.Second)
	defer timer.Stop()
	<-timer.C

	original, _, _, err := flow.Pack(proposer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	con := New(c, stateCreator)
	if _, _, err := con.Process(original, flow.When()); err != nil {
		t.Fatal(err)
	}

	return &testConsensus{
		t:        t,
		con:      con,
		time:     flow.When(),
		pk:       proposer.PrivateKey,
		parent:   parent,
		original: original,
	}
}

func (tc *testConsensus) sign(blk *block.Block) *block.Block {
	sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), tc.pk)
	if err != nil {
		tc.t.Fatal(err)
	}
	return blk.WithSignature(sig)
}

func (tc *testConsensus) originalBuilder() *block.Builder {
	header := tc.original.Header()
	return new(block.Builder).
		ParentID(header.ParentID()).
		Timestamp(header.Timestamp() + 1).
		TotalScore(header.TotalScore()).
		GasLimit(header.GasLimit()).
		GasUsed(header.GasUsed()).
		Beneficiary(header.Beneficiary()).
		StateRoot(header.StateRoot()).
		ReceiptsRoot(header.ReceiptsRoot())
}

func (tc *testConsensus) consent(blk *block.Block) error {
	_, _, err := tc.con.Process(blk, tc.time)
	return err
}
