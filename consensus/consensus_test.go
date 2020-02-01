// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"crypto/ecdsa"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vrf"
)

func TestConsensus(t *testing.T) {
	ob := newTestConsensus(t)
	ob.newBlock(2, nil)
	ob.commitNewBlock()
	ob.newBlock(3, nil)

	obValue := reflect.ValueOf(ob)
	obType := obValue.Type()
	for i := 0; i < obValue.NumMethod(); i++ {
		if strings.HasPrefix(obType.Method(i).Name, "Test") {
			obValue.Method(i).Call(nil)
		}
	}
}

func txBuilder(tag byte) *tx.Builder {
	address := thor.BytesToAddress([]byte("addr"))
	return new(tx.Builder).
		GasPriceCoef(1).
		Gas(1000000).
		Expiration(100).
		Clause(tx.NewClause(&address).WithValue(big.NewInt(10)).WithData(nil)).
		Nonce(1).
		ChainTag(tag)
}

func txSign(builder *tx.Builder, sk *ecdsa.PrivateKey) *tx.Transaction {
	transaction := builder.Build()
	sig, _ := crypto.Sign(transaction.SigningHash().Bytes(), sk)
	return transaction.WithSignature(sig)
}

type testConsensus struct {
	t            *testing.T
	assert       *assert.Assertions
	con          *Consensus
	time         uint64
	tag          byte
	original     *block.Block
	stage        *state.Stage
	receipts     tx.Receipts
	proposer     *account
	parent       *block.Block
	nodes        []*account
	genesisBlock *block.Block
	chain        *chain.Chain
	stateCreator *state.Creator
}

type account struct {
	ethsk *ecdsa.PrivateKey
	addr  thor.Address
	vrfsk *vrf.PrivateKey
	vrfpk *vrf.PublicKey
}

// generate thor.MaxBlockProposers key pairs and register them as master nodes
func newTestConsensus(t *testing.T) *testConsensus {
	db := muxdb.NewMem()

	var accs []*account
	for i := uint64(0); i < thor.MaxBlockProposers; i++ {
		ethsk, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(ethsk.PublicKey)
		vrfpk, vrfsk := vrf.GenKeyPair()
		accs = append(accs, &account{ethsk, thor.BytesToAddress(addr.Bytes()), vrfsk, vrfpk})
	}

	launchTime := uint64(1526400000)
	gen := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(launchTime).
		State(func(state *state.State) error {
			bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(genesis.DevAccounts()[0].Address[:]))
			// for _, acc := range genesis.DevAccounts() {
			for _, acc := range accs {
				state.SetBalance(acc.addr, bal)
				state.SetEnergy(acc.addr, bal, launchTime)

				builtin.Authority.Native(state).Add(acc.addr, acc.addr, thor.Bytes32{}, acc.vrfpk.Bytes32())
			}
			return nil
		})

	stateCreator := state.NewCreator(db)
	genesisBlock, _, err := gen.Build(stateCreator)
	if err != nil {
		t.Fatal(err)
	}

	c, err := chain.New(db, genesisBlock)
	if err != nil {
		t.Fatal(err)
	}

	forkConfig := thor.ForkConfig{
		VIP191:    math.MaxUint32,
		ETH_CONST: math.MaxUint32,
		BLOCKLIST: 0,
	}
	con := New(c, stateCreator, forkConfig)

	return &testConsensus{
		t:            t,
		assert:       assert.New(t),
		con:          con,
		nodes:        accs,
		tag:          c.Tag(),
		chain:        c,
		stateCreator: stateCreator,
		genesisBlock: genesisBlock,
	}
}

// create a new block without committing to the state
func (tc *testConsensus) newBlock(round uint32, txs []*tx.Transaction) {
	var (
		flow     *packer.Flow
		proposer *account
		err      error
	)

	now := tc.con.Timestamp(round)
	parent := tc.chain.BestBlock()

	if now < parent.Header().Timestamp() {
		tc.t.Fatal("new block earlier than the best block")
	}

	// search for the legit proposer
	for _, acc := range tc.nodes {
		p := packer.New(tc.chain, tc.stateCreator, acc.addr, &acc.addr, thor.NoFork)
		flow, err = p.Schedule(parent.Header(), now)
		if err != nil {
			continue
		}

		if flow.When() == now {
			proposer = acc
			break
		}
		flow = nil
	}
	if flow == nil {
		tc.t.Fatal("No proposer found")
	}

	// add transactions
	for _, tx := range txs {
		flow.Adopt(tx)
	}

	// pack block summary
	bs, _, err := flow.PackTxSetAndBlockSummary(proposer.ethsk)
	if err != nil {
		tc.t.Fatal(err)
	}

	// pack endorsements
	for _, acc := range tc.nodes {
		if ok, proof, _ := tc.con.IsCommittee(acc.vrfsk, now); ok {
			ed := block.NewEndorsement(bs, proof)
			sig, _ := crypto.Sign(ed.SigningHash().Bytes(), acc.ethsk)
			ed = ed.WithSignature(sig)
			flow.AddEndoresement(ed)
		}
		if uint64(flow.NumOfEndorsements()) >= thor.CommitteeSize {
			break
		}
	}
	if uint64(flow.NumOfEndorsements()) < thor.CommitteeSize {
		tc.t.Errorf("Not enough endorsements added")
	}

	// pack block
	newBlock, stage, receipts, err := flow.Pack(proposer.ethsk)
	if err != nil {
		tc.t.Fatal(err)
	}

	// validate block
	if _, _, err := tc.con.Process(newBlock, flow.When()); err != nil {
		tc.t.Fatal(err)
	}

	tc.parent = parent
	tc.time = now
	tc.original = newBlock
	tc.proposer = proposer
	tc.stage = stage
	tc.receipts = receipts
}

func (tc *testConsensus) commitNewBlock() {
	if _, err := tc.chain.GetBlockHeader(tc.original.Header().ID()); err == nil {
		tc.t.Fatal("known in-chain block")
	}

	if _, err := tc.stage.Commit(); err != nil {
		tc.t.Fatal(err)
	}

	_, err := tc.chain.AddBlock(tc.original, tc.receipts)
	if err != nil {
		tc.t.Fatal(err)
	}
}

func (tc *testConsensus) sign(blk *block.Block) *block.Block {
	sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), tc.proposer.ethsk)
	if err != nil {
		tc.t.Fatal(err)
	}
	return blk.WithSignature(sig)
}

/**
 * rebuild takes the current block builder and re-compute the block summary
 * and the endorsements. It then update the builder with the correct
 * signatures and vrf proofs
 */
func (tc *testConsensus) rebuild(builder *block.Builder) *block.Builder {
	blk := builder.Build()
	header := blk.Header()

	// rebuild block summary
	bs := block.NewBlockSummary(
		header.ParentID(),
		header.TxsRoot(),
		header.Timestamp(),
		header.TotalScore())
	sig, err := crypto.Sign(bs.SigningHash().Bytes(), tc.proposer.ethsk)
	if err != nil {
		tc.t.Fatal(err)
	}
	bs = bs.WithSignature(sig)

	var (
		sigs   [][]byte
		proofs []*vrf.Proof
		N      = int(thor.CommitteeSize)
	)

	// rebuild endorsements
	for _, acc := range tc.nodes {
		if ok, proof, err := tc.con.IsCommittee(acc.vrfsk, header.Timestamp()); ok {
			ed := block.NewEndorsement(bs, proof)
			sig, _ := crypto.Sign(ed.SigningHash().Bytes(), acc.ethsk)
			proofs = append(proofs, proof)
			sigs = append(sigs, sig)
		} else if err != nil {
			tc.t.Fatal(err)
		}
		if len(proofs) >= N {
			break
		}
	}
	if len(sigs) != N {
		tc.t.Fatal("Not enough endorsements collected")
	}

	newBuilder := new(block.Builder).
		ParentID(header.ParentID()).
		Timestamp(header.Timestamp()).
		TotalScore(header.TotalScore()).
		GasLimit(header.GasLimit()).
		GasUsed(header.GasUsed()).
		Beneficiary(header.Beneficiary()).
		StateRoot(header.StateRoot()).
		ReceiptsRoot(header.ReceiptsRoot()).
		TransactionFeatures(header.TxsFeatures()).
		// update signatures and vrf proofs
		SigOnBlockSummary(sig).
		SigsOnEndorsement(sigs).
		VrfProofs(proofs)

	// add existing transactions
	for _, tx := range blk.Transactions() {
		newBuilder.Transaction(tx)
	}

	return newBuilder
}

func (tc *testConsensus) originalBuilder() *block.Builder {
	header := tc.original.Header()
	return new(block.Builder).
		ParentID(header.ParentID()).
		Timestamp(header.Timestamp()).
		TotalScore(header.TotalScore()).
		GasLimit(header.GasLimit()).
		GasUsed(header.GasUsed()).
		Beneficiary(header.Beneficiary()).
		StateRoot(header.StateRoot()).
		ReceiptsRoot(header.ReceiptsRoot()).
		TransactionFeatures(header.TxsFeatures()).
		SigOnBlockSummary(header.SigOnBlockSummary()).
		SigsOnEndorsement(header.SigsOnEndoresment()).
		VrfProofs(header.VrfProofs())
}

func (tc *testConsensus) consent(blk *block.Block) error {
	_, _, err := tc.con.Process(blk, tc.time)
	return err
}

func (tc *testConsensus) TestValidateBlockHeader() {
	triggers := make(map[string]func())
	triggers["triggerErrTimestampBehindParent"] = func() {
		build := tc.originalBuilder()

		blk := tc.sign(
			tc.rebuild(
				build.Timestamp(tc.parent.Header().Timestamp())).Build())
		err := tc.consent(blk)
		// expect := consensusError(
		// 	fmt.Sprintf(
		// 		"block timestamp behind parents: parent %v, current %v",
		// 		tc.parent.Header().Timestamp(),
		// 		blk.Header().Timestamp(),
		// 	),
		// )
		expect := newConsensusError(trHeader, strErrTimestamp,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.parent.Header().Timestamp(), blk.Header().Timestamp()}, "").Error()
		tc.assert.Equal(expect, err.Error())

		blk = tc.sign(
			tc.rebuild(
				build.Timestamp(tc.parent.Header().Timestamp() - 1)).Build())
		err = tc.consent(blk)
		// expect = consensusError(
		// 	fmt.Sprintf(
		// 		"block timestamp behind parents: parent %v, current %v",
		// 		tc.parent.Header().Timestamp(),
		// 		blk.Header().Timestamp(),
		// 	),
		// )
		expect = newConsensusError(trHeader, strErrTimestamp,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.parent.Header().Timestamp(), blk.Header().Timestamp()}, "").Error()
		tc.assert.Equal(expect, err.Error())
	}
	triggers["triggerErrInterval"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(
			tc.rebuild(
				build.Timestamp(tc.original.Header().Timestamp() + 1)).Build())
		err := tc.consent(blk)
		// expect := consensusError(
		// 	fmt.Sprintf(
		// 		"block interval not rounded: parent %v, current %v",
		// 		tc.parent.Header().Timestamp(),
		// 		blk.Header().Timestamp(),
		// 	),
		// )
		expect := newConsensusError(trHeader, strErrTimestamp,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.parent.Header().Timestamp(), blk.Header().Timestamp()}, "").Error()
		tc.assert.Equal(expect, err.Error())
	}
	triggers["triggerErrFutureBlock"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(
			tc.rebuild(
				build.Timestamp(tc.time + thor.BlockInterval*2)).Build())
		err := tc.consent(blk)
		tc.assert.Equal(errFutureBlock, err)
	}
	triggers["triggerInvalidGasLimit"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.GasLimit(tc.parent.Header().GasLimit() * 2).Build())
		err := tc.consent(blk)
		expect := newConsensusError(
			trHeader,
			strErrGasLimit,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.parent.Header().GasLimit(), blk.Header().GasLimit()}, "").Error()
		tc.assert.Equal(expect, err.Error())
	}
	triggers["triggerExceedGaUsed"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.GasUsed(tc.original.Header().GasLimit() + 1).Build())
		err := tc.consent(blk)
		expect := newConsensusError(
			trHeader,
			strErrGasExceed,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{tc.parent.Header().GasLimit(), blk.Header().GasUsed()}, "").Error()
		tc.assert.Equal(expect, err.Error())
	}
	triggers["triggerInvalidTotalScore"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(
			tc.rebuild(build.TotalScore(tc.parent.Header().TotalScore())).Build())
		err := tc.consent(blk)
		expect := newConsensusError(trHeader, strErrTotalScore,
			[]string{strDataParent, strDataCurr},
			[]interface{}{tc.parent.Header().TotalScore(), blk.Header().TotalScore()}, "").Error()
		tc.assert.Equal(expect, err.Error())
	}

	for _, trigger := range triggers {
		trigger()
	}
}

func (tc *testConsensus) TestTxDepBroken() {
	txID := txSign(txBuilder(tc.tag), tc.nodes[1].ethsk).ID()
	tx := txSign(txBuilder(tc.tag).DependsOn(&txID), tc.proposer.ethsk)
	err := tc.consent(
		tc.sign(
			tc.rebuild(tc.originalBuilder().Transaction(tx)).Build(),
		),
	)
	expect := newConsensusError("verifyBlock: ", "tx dep broken", nil, nil, "").Error()
	tc.assert.Equal(expect, err.Error())
}

func (tc *testConsensus) TestKnownBlock() {
	err := tc.consent(tc.parent)
	tc.assert.Equal(err, errKnownBlock)
}

func (tc *testConsensus) TestTxAlreadyExists() {
	tx := txSign(txBuilder(tc.tag), tc.proposer.ethsk)

	builder := tc.originalBuilder().Transaction(tx).Transaction(tx)
	builder = tc.rebuild(builder)
	blk := builder.Build()
	blk = tc.sign(blk)

	err := tc.consent(blk)
	expect := newConsensusError("verifyBlock: ", "tx already exists", nil, nil, "").Error()
	tc.assert.Equal(expect, err.Error())
}

func (tc *testConsensus) TestParentMissing() {
	build := tc.originalBuilder()
	blk := tc.sign(
		tc.rebuild(build.ParentID(tc.original.Header().ID())).Build())
	err := tc.consent(blk)
	tc.assert.Equal(errParentMissing, err)
}

func (tc *testConsensus) TestValidateBlockBody() {
	triggers := make(map[string]func())
	triggers["triggerErrTxSignerUnavailable"] = func() {
		blk := tc.sign(
			tc.rebuild(
				tc.originalBuilder().
					Transaction(txBuilder(tc.tag).
						Build()),
			).Build())
		err := tc.consent(blk)
		// expect := consensusError("tx signer unavailable: invalid signature length")
		expect := newConsensusError(trBlockBody, strErrSignature, nil, nil, "invalid signature length").Error()
		tc.assert.Equal(expect, err.Error())
	}

	triggers["triggerErrTxsRootMismatch"] = func() {
		transaction := txSign(txBuilder(tc.tag), tc.nodes[1].ethsk)
		transactions := tx.Transactions{transaction}
		blk := tc.sign(block.Compose(tc.original.Header(), transactions))
		err := tc.consent(blk)
		// expect := consensusError(
		// 	fmt.Sprintf(
		// 		"block txs root mismatch: want %v, have %v",
		// 		tc.original.Header().TxsRoot(),
		// 		transactions.RootHash(),
		// 	),
		// )
		expect := newConsensusError(trBlockBody, strErrTxsRoot,
			[]string{strDataExpected, strDataCurr},
			[]interface{}{tc.original.Header().TxsRoot(), transactions.RootHash()}, "").Error()
		tc.assert.Equal(expect, err.Error())
	}
	triggers["triggerErrChainTagMismatch"] = func() {
		err := tc.consent(
			tc.sign(
				tc.rebuild(
					tc.originalBuilder().
						Transaction(txSign(txBuilder(tc.tag+1), tc.nodes[1].ethsk)),
				).Build()))
		// expect := consensusError(
		// 	fmt.Sprintf(
		// 		"tx chain tag mismatch: want %v, have %v",
		// 		tc.tag,
		// 		tc.tag+1,
		// 	),
		// )
		expect := strErrChainTag
		tc.assert.Equal(expect, err.(consensusError).ErrorMsg())
	}
	triggers["triggerErrRefFutureBlock"] = func() {
		blk := tc.sign(
			tc.rebuild(
				tc.originalBuilder().
					Transaction(txSign(txBuilder(tc.tag).
						BlockRef(tx.NewBlockRef(100)), tc.nodes[1].ethsk)),
			).Build())
		err := tc.consent(blk)

		// expect := consensusError("tx ref future block: ref 100, current 1")
		expect := newConsensusError(trBlockBody, strErrFutureTx,
			[]string{strDataRef, strDataCurr},
			[]interface{}{uint32(100), blk.Header().Number()}, "").Error()
		tc.assert.Equal(expect, err.Error())
	}
	triggers["triggerTxOriginBlocked"] = func() {
		thor.MockBlocklist([]string{genesis.DevAccounts()[9].Address.String()})
		t := txBuilder(tc.tag).Build()
		sig, _ := crypto.Sign(t.SigningHash().Bytes(), genesis.DevAccounts()[9].PrivateKey)
		t = t.WithSignature(sig)

		blk := tc.sign(
			tc.rebuild(tc.originalBuilder().Transaction(t)).Build(),
		)
		err := tc.consent(blk)
		// expect := consensusError(
		// 	fmt.Sprintf("tx origin blocked got packed: %v", genesis.DevAccounts()[9].Address),
		// )
		expect := newConsensusError(trBlockBody, strErrBlockedTxOrign,
			[]string{strDataAddr}, []interface{}{genesis.DevAccounts()[9].Address}, "")
		tc.assert.Equal(err, expect)
	}

	for _, trigger := range triggers {
		trigger()
	}
}

func (tc *testConsensus) TestValidateProposer() {
	triggers := make(map[string]func())
	triggers["triggerErrSignerUnavailable"] = func() {
		blk := tc.originalBuilder().Build()
		err := tc.consent(blk)
		// expect := consensusError("block signer unavailable: invalid signature length")
		expect := newConsensusError(trProposer, strErrSignature, nil, nil, "invalid signature length").Error()
		tc.assert.Equal(expect, err.Error())
	}
	triggers["triggerErrSignerInvalid"] = func() {
		blk := tc.originalBuilder().Build()
		sk, _ := crypto.GenerateKey()
		sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), sk)
		blk = blk.WithSignature(sig)
		err := tc.consent(blk)
		// expect := consensusError(
		// 	fmt.Sprintf(
		// 		"block signer invalid: %v unauthorized block proposer",
		// 		thor.Address(crypto.PubkeyToAddress(pk.PublicKey)),
		// 	),
		// )
		signer, _ := blk.Header().Signer()
		expect := newConsensusError(trProposer, strErrSigner,
			[]string{strDataAddr},
			[]interface{}{signer}, "unauthorized block proposer").Error()
		tc.assert.Equal(expect, err.Error())
	}
	triggers["triggerErrTimestampUnscheduled"] = func() {
		blk := tc.originalBuilder().Build()
		sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), genesis.DevAccounts()[1].PrivateKey)
		blk = blk.WithSignature(sig)
		err := tc.consent(blk)
		// expect := consensusError(
		// 	fmt.Sprintf(
		// 		"block timestamp unscheduled: t %v, s %v",
		// 		blk.Header().Timestamp(),
		// 		thor.Address(crypto.PubkeyToAddress(genesis.DevAccounts()[1].PrivateKey.PublicKey)),
		// 	),
		// )
		expect := newConsensusError(trProposer, strErrSigner,
			[]string{strDataAddr},
			[]interface{}{thor.Address(crypto.PubkeyToAddress(genesis.DevAccounts()[1].PrivateKey.PublicKey))},
			"unauthorized block proposer").Error()
		tc.assert.Equal(expect, err.Error())
	}
	/**
	 * The test below is removed since the total score is used to reconstruct and validate
	 * the block summary. The validation would fail if the score is incorrect.
	 */
	// triggers["triggerTotalScoreInvalid"] = func() {
	// 	build := tc.originalBuilder()
	// 	blk := tc.sign(
	// 		build.TotalScore(tc.original.Header().TotalScore() + 100).Build())
	// 	err := tc.consent(blk)
	// 	// expect := consensusError("block total score invalid: want 1, have 101")
	// 	expect := newConsensusError(trHeader, strErrTotalScore,
	// 		[]string{strDataParent, strDataCurr},
	// 		[]interface{}{uint64(1), uint64(101)}, "").Error()
	// 	fmt.Println(expect)
	// 	fmt.Println(err.Error())
	// 	tc.assert.Equal(expect, err.Error())
	// }

	for _, trigger := range triggers {
		trigger()
	}
}
