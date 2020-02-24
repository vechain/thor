// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/builtin/authority"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestConsensus(t *testing.T) {
	ob, err := newTestConsensus(t, 10)
	if err != nil {
		t.Fatal(err)
	}
	ob.NewBlock(2, nil)
	ob.CommitNewBlock()
	ob.NewBlock(3, nil)

	obValue := reflect.ValueOf(ob)
	obType := obValue.Type()
	for i := 0; i < obValue.NumMethod(); i++ {
		if strings.HasPrefix(obType.Method(i).Name, "Test") {
			obValue.Method(i).Call(nil)
		}
	}
}

type testConsensus struct {
	*TempChain
	t *testing.T
}

func newTestConsensus(t *testing.T, N int) (*testConsensus, error) {
	tc, err := NewTempChain(N, thor.ForkConfig{}) // enabling vip191, ethconst and blocklist
	if err != nil {
		return nil, err
	}
	return &testConsensus{tc, t}, nil
}

func (tc *testConsensus) signAndVerifyBlock(b *block.Block) error {
	blk, err := tc.Sign(b)
	if err != nil {
		tc.t.Fatal(err)
	}
	if err := tc.Consent(blk); err != nil {
		return err
	}
	return nil
}

func (tc *testConsensus) TestValidateBlockHeader() {
	triggers := make(map[string]func())
	triggers["triggerErrTimestampBehindParent"] = func() {
		build := tc.OriginalBuilder()
		rebuild, err := tc.Rebuild(build.Timestamp(tc.Parent.Header().Timestamp()))
		if err != nil {
			tc.t.Fatal(err)
		}
		blk := rebuild.Build()
		actual := tc.signAndVerifyBlock(blk).Error()

		// expected := consensusError(
		// 	fmt.Sprintf(
		// 		"block timestamp behind parents: parent %v, current %v",
		// 		tc.Parent.Header().Timestamp(),
		// 		blk.Header().Timestamp(),
		// 	),
		// )
		expected := newConsensusError(trHeader, strErrTimestamp,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.Parent.Header().Timestamp(), blk.Header().Timestamp()}, "").Error()
		assert.Equal(tc.t, actual, expected)

		rebuild, err = tc.Rebuild(build.Timestamp(tc.Parent.Header().Timestamp() - 1))
		if err != nil {
			tc.t.Fatal(err)
		}
		blk = rebuild.Build()
		actual = tc.signAndVerifyBlock(blk).Error()
		// expected = consensusError(
		// 	fmt.Sprintf(
		// 		"block timestamp behind parents: parent %v, current %v",
		// 		tc.Parent.Header().Timestamp(),
		// 		blk.Header().Timestamp(),
		// 	),
		// )
		expected = newConsensusError(trHeader, strErrTimestamp,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.Parent.Header().Timestamp(), blk.Header().Timestamp()}, "").Error()
		assert.Equal(tc.t, expected, actual)
	}
	triggers["triggerErrInterval"] = func() {
		build := tc.OriginalBuilder()
		rebuild, err := tc.Rebuild(build.Timestamp(tc.Original.Header().Timestamp() + 1))
		if err != nil {
			tc.t.Fatal(err)
		}
		blk := rebuild.Build()
		actual := tc.signAndVerifyBlock(blk).Error()
		// expected := consensusError(
		// 	fmt.Sprintf(
		// 		"block interval not rounded: parent %v, current %v",
		// 		tc.Parent.Header().Timestamp(),
		// 		blk.Header().Timestamp(),
		// 	),
		// )
		expected := newConsensusError(trHeader, strErrTimestamp,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.Parent.Header().Timestamp(), blk.Header().Timestamp()}, "").Error()
		assert.Equal(tc.t, expected, actual)
	}
	triggers["triggerErrFutureBlock"] = func() {
		build := tc.OriginalBuilder()
		rebuild, err := tc.Rebuild(build.Timestamp(tc.Time + thor.BlockInterval*2))
		if err != nil {
			tc.t.Fatal(err)
		}
		err = tc.signAndVerifyBlock(rebuild.Build())
		assert.Equal(tc.t, errFutureBlock, err)
	}
	triggers["triggerInvalidGasLimit"] = func() {
		build := tc.OriginalBuilder().GasLimit(tc.Parent.Header().GasLimit() * 2)
		blk := build.Build()
		actual := tc.signAndVerifyBlock(blk).Error()
		expected := newConsensusError(
			trHeader,
			strErrGasLimit,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.Parent.Header().GasLimit(), blk.Header().GasLimit()}, "").Error()
		assert.Equal(tc.t, expected, actual)
	}
	triggers["triggerExceedGaUsed"] = func() {
		build := tc.OriginalBuilder().GasUsed(tc.Original.Header().GasLimit() + 1)
		blk := build.Build()
		actual := tc.signAndVerifyBlock(blk).Error()
		expected := newConsensusError(
			trHeader,
			strErrGasExceed,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{tc.Parent.Header().GasLimit(), blk.Header().GasUsed()}, "").Error()
		assert.Equal(tc.t, expected, actual)
	}
	triggers["triggerInvalidTotalScore"] = func() {
		build := tc.OriginalBuilder().TotalScore(tc.Parent.Header().TotalScore())
		blk := build.Build()
		actual := tc.signAndVerifyBlock(blk).Error()
		expected := newConsensusError(trHeader, strErrTotalScore,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.Parent.Header().TotalScore(), blk.Header().TotalScore()}, "").Error()
		assert.Equal(tc.t, expected, actual)
	}

	for _, trigger := range triggers {
		trigger()
	}
}

func (tc *testConsensus) TestTxDepBroken() {
	txID := TxSign(TxBuilder(tc.Tag), tc.Nodes[1].Ethsk).ID()
	tx := TxSign(TxBuilder(tc.Tag).DependsOn(&txID), tc.Proposer.Ethsk)
	build, err := tc.Rebuild(tc.OriginalBuilder().Transaction(tx))
	if err != nil {
		tc.t.Fatal(err)
	}
	actual := tc.signAndVerifyBlock(build.Build()).Error()
	expected := newConsensusError("verifyBlock: ", "tx dep broken", nil, nil, "").Error()
	assert.Equal(tc.t, expected, actual)
}

func (tc *testConsensus) TestKnownBlock() {
	err := tc.Consent(tc.Parent)
	assert.Equal(tc.t, err, errKnownBlock)
}

func (tc *testConsensus) TestTxAlreadyExists() {
	tx := TxSign(TxBuilder(tc.Tag), tc.Proposer.Ethsk)
	build, err := tc.Rebuild(tc.OriginalBuilder().Transaction(tx).Transaction(tx))
	if err != nil {
		tc.t.Fatal(err)
	}
	actual := tc.signAndVerifyBlock(build.Build()).Error()
	expected := newConsensusError("verifyBlock: ", "tx already exists", nil, nil, "").Error()
	assert.Equal(tc.t, expected, actual)
}

func (tc *testConsensus) TestParentMissing() {
	build := tc.OriginalBuilder().ParentID(tc.Original.Header().ID())
	err := tc.signAndVerifyBlock(build.Build())
	assert.Equal(tc.t, errParentMissing, err)
}

func (tc *testConsensus) TestValidateBlockBody() {
	triggers := make(map[string]func())
	triggers["triggerErrTxSignerUnavailable"] = func() {
		build, err := tc.Rebuild(tc.OriginalBuilder().Transaction(TxBuilder(tc.Tag).Build()))
		if err != nil {
			tc.t.Fatal(err)
		}
		actual := tc.signAndVerifyBlock(build.Build()).Error()
		// expected := consensusError("tx signer unavailable: invalid signature length")
		expected := newConsensusError(trBlockBody, strErrSignature, nil, nil, "invalid signature length").Error()
		assert.Equal(tc.t, expected, actual)
	}

	triggers["triggerErrTxsRootMismatch"] = func() {
		transaction := TxSign(TxBuilder(tc.Tag), tc.Nodes[1].Ethsk)
		transactions := tx.Transactions{transaction}
		actual := tc.signAndVerifyBlock(block.Compose(tc.Original.Header(), transactions)).Error()
		// expected := consensusError(
		// 	fmt.Sprintf(
		// 		"block txs root mismatch: want %v, have %v",
		// 		tc.Original.Header().TxsRoot(),
		// 		transactions.RootHash(),
		// 	),
		// )
		expected := newConsensusError(trBlockBody, strErrTxsRoot,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{tc.Original.Header().TxsRoot(), transactions.RootHash()}, "").Error()
		assert.Equal(tc.t, expected, actual)
	}
	triggers["triggerErrChainTagMismatch"] = func() {
		build, err := tc.Rebuild(tc.OriginalBuilder().
			Transaction(TxSign(TxBuilder(tc.Tag+1), tc.Nodes[1].Ethsk)))
		if err != nil {
			tc.t.Fatal(err)
		}
		actual := tc.signAndVerifyBlock(build.Build()).(consensusError).ErrorMsg()
		// expected := consensusError(
		// 	fmt.Sprintf(
		// 		"tx chain tag mismatch: want %v, have %v",
		// 		tc.Tag,
		// 		tc.Tag+1,
		// 	),
		// )
		expected := strErrChainTag
		assert.Equal(tc.t, expected, actual)
	}
	triggers["triggerErrRefFutureBlock"] = func() {
		build, err := tc.Rebuild(tc.OriginalBuilder().
			Transaction(TxSign(TxBuilder(tc.Tag).BlockRef(tx.NewBlockRef(100)), tc.Nodes[1].Ethsk)))
		if err != nil {
			tc.t.Fatal(err)
		}
		blk := build.Build()
		actual := tc.signAndVerifyBlock(blk).Error()

		// expected := consensusError("tx ref future block: ref 100, current 1")
		expected := newConsensusError(trBlockBody, strErrFutureTx,
			[]string{strDataRef, strDataCurr},
			[]interface{}{uint32(100), blk.Header().Number()}, "").Error()
		assert.Equal(tc.t, expected, actual)
	}
	triggers["triggerTxOriginBlocked"] = func() {
		thor.MockBlocklist([]string{genesis.DevAccounts()[9].Address.String()})
		t := TxBuilder(tc.Tag).Build()
		sig, _ := crypto.Sign(t.SigningHash().Bytes(), genesis.DevAccounts()[9].PrivateKey)
		t = t.WithSignature(sig)

		build, err := tc.Rebuild(tc.OriginalBuilder().Transaction(t))
		if err != nil {
			tc.t.Fatal(err)
		}
		actual := tc.signAndVerifyBlock(build.Build()).Error()
		// expected := consensusError(
		// 	fmt.Sprintf("tx origin blocked got packed: %v", genesis.DevAccounts()[9].Address),
		// )
		expected := newConsensusError(trBlockBody, strErrBlockedTxOrign,
			[]string{strDataAddr}, []interface{}{genesis.DevAccounts()[9].Address}, "").Error()
		assert.Equal(tc.t, expected, actual)
	}

	for _, trigger := range triggers {
		trigger()
	}
}

func (tc *testConsensus) TestValidateProposer() {
	triggers := make(map[string]func())
	triggers["triggerErrSignerUnavailable"] = func() {
		blk := tc.OriginalBuilder().Build()
		actual := tc.Consent(blk).Error()
		// expected := consensusError("block signer unavailable: invalid signature length")
		expected := newConsensusError(trProposer, strErrSignature, nil, nil, "invalid signature length").Error()
		assert.Equal(tc.t, expected, actual)
	}
	triggers["triggerErrSignerInvalid"] = func() {
		blk := tc.OriginalBuilder().Build()
		sk, _ := crypto.GenerateKey()
		sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), sk)
		blk = blk.WithSignature(sig)
		actual := tc.Consent(blk).Error()
		// expected := consensusError(
		// 	fmt.Sprintf(
		// 		"block signer invalid: %v unauthorized block Proposer",
		// 		thor.Address(crypto.PubkeyToAddress(pk.PublicKey)),
		// 	),
		// )
		signer, _ := blk.Header().Signer()
		expected := newConsensusError(trProposer, strErrSigner,
			[]string{strDataAddr},
			[]interface{}{signer}, "unauthorized block proposer").Error()
		assert.Equal(tc.t, expected, actual)
	}
	triggers["triggerErrTimestampUnscheduled"] = func() {
		blk := tc.OriginalBuilder().Build()
		sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), genesis.DevAccounts()[1].PrivateKey)
		blk = blk.WithSignature(sig)
		actual := tc.Consent(blk).Error()
		// expected := consensusError(
		// 	fmt.Sprintf(
		// 		"block timestamp unscheduled: t %v, s %v",
		// 		blk.Header().Timestamp(),
		// 		thor.Address(crypto.PubkeyToAddress(genesis.DevAccounts()[1].PrivateKey.PublicKey)),
		// 	),
		// )
		expected := newConsensusError(trProposer, strErrSigner,
			[]string{strDataAddr},
			[]interface{}{thor.Address(crypto.PubkeyToAddress(genesis.DevAccounts()[1].PrivateKey.PublicKey))},
			"unauthorized block proposer").Error()
		assert.Equal(tc.t, expected, actual)
	}
	/**
	 * The test below is commented since the total score is used to reconstruct and validate
	 * the block summary. The validation would fail if the score is incorrect.
	 */
	// triggers["triggerTotalScoreInvalid"] = func() {
	// 	build := tc.OriginalBuilder()
	// 	blk := tc.Sign(
	// 		build.TotalScore(tc.Original.Header().TotalScore() + 100).Build())
	// 	actual := tc.Consent(blk).Error()
	// 	// expected := consensusError("block total score invalid: want 1, have 101")
	// 	expected := newConsensusError(trHeader, strErrTotalScore,
	// 		[]string{strDataParent, strDataCurr},
	// 		[]interface{}{uint64(1), uint64(101)}, "").Error()
	// 	fmt.Println(expected)
	// 	fmt.Println(err.Error())
	// 	assert.Equal(tc.t,expected, actual)
	// }

	for _, trigger := range triggers {
		trigger()
	}
}

func TestUpdateConsensusNodeForVip193(t *testing.T) {
	candidates := []authority.Candidate{
		{
			thor.MustParseAddress("0x2a02604a8b7aaa84991c21d7de1c3238046c5275"),
			thor.MustParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed"),
			thor.Bytes32{},
			true,
			thor.MustParseBytes32("0x96893d6f2d785dbdf75d635d74ee53b85a3e7837150d321c4965de3def134182"),
		},
		{
			thor.MustParseAddress("0x86fd9eb1cf082d7d6b0c6033fc89ccfcbf648549"),
			thor.MustParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed"),
			thor.Bytes32{},
			true,
			thor.MustParseBytes32("0x97b182c4d88435c3781bf5f29a59c169a91564acbf193c9ba95a4db3fa703f26"),
		},
		{
			thor.MustParseAddress("0x8f53d18bb03c84ed92abe0b6a9a8c277dbbf719f"),
			thor.MustParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed"),
			thor.Bytes32{},
			true,
			thor.MustParseBytes32("0x2ab534b885f45e7e628e3bea8bb1a7e914f0009d077a44ac2d4461e7731fcb2c"),
		},
	}

	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gen, _, _, err := new(genesis.Builder).State(func(st *state.State) error {
		st.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())

		aut := builtin.Authority.Native(st)
		for _, c := range candidates {
			ok, err := aut.Add(c.NodeMaster, c.Endorsor, c.Identity)
			assert.True(t, ok)
			assert.Nil(t, err)
		}
		return nil
	}).Build(stater)
	assert.Nil(t, err)

	repo, err := chain.NewRepository(db, gen)
	assert.Nil(t, err)
	cons := New(repo, stater, thor.ForkConfig{
		VIP193: 1,
	})

	header := new(block.Builder).ParentID(thor.Bytes32{}).Build().Header()

	err = cons.UpdateConsensusNodesForVip193(header)
	assert.Nil(t, err)
}
