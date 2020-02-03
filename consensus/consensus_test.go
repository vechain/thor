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
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestConsensus(t *testing.T) {
	ob, err := newTestConsensus(t)
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

func newTestConsensus(t *testing.T) (*testConsensus, error) {
	tc, err := NewTempChain(thor.ForkConfig{0, 0, 0}) // enabling vip191, ethconst and blocklist
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

// func TxBuilder(tag byte) *tx.Builder {
// 	address := thor.BytesToAddress([]byte("addr"))
// 	return new(tx.Builder).
// 		GasPriceCoef(1).
// 		Gas(1000000).
// 		Expiration(100).
// 		Clause(tx.NewClause(&address).WithValue(big.NewInt(10)).WithData(nil)).
// 		Nonce(1).
// 		ChainTag(tag)
// }

// func TxSign(builder *tx.Builder, sk *ecdsa.PrivateKey) *tx.Transaction {
// 	transaction := builder.Build()
// 	sig, _ := crypto.Sign(transaction.SigningHash().Bytes(), sk)
// 	return transaction.WithSignature(sig)
// }

// type testConsensus struct {
// 	t            *testing.T
// 	assert       *assert.Assertions
// 	con          *Consensus
// 	time         uint64
// 	tag          byte
// 	original     *block.Block
// 	stage        *state.Stage
// 	receipts     tx.Receipts
// 	Proposer     *account
// 	parent       *block.Block
// 	nodes        []*account
// 	genesisBlock *block.Block
// 	chain        *chain.Chain
// 	stateCreator *state.Creator
// }

// type account struct {
// 	Ethsk *ecdsa.PrivateKey
// 	addr  thor.Address
// 	vrfsk *vrf.PrivateKey
// 	vrfpk *vrf.PublicKey
// }

// // generate thor.MaxBlockProposers key pairs and register them as master nodes
// func newTestConsensus(t *testing.T) *testConsensus {
// 	db, err := lvldb.NewMem()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	var accs []*account
// 	for i := uint64(0); i < thor.MaxBlockProposers; i++ {
// 		Ethsk, _ := crypto.GenerateKey()
// 		addr := crypto.PubkeyToAddress(Ethsk.PublicKey)
// 		vrfpk, vrfsk := vrf.GenKeyPair()
// 		accs = append(accs, &account{Ethsk, thor.BytesToAddress(addr.Bytes()), vrfsk, vrfpk})
// 	}

// 	launchTime := uint64(1526400000)
// 	gen := new(genesis.Builder).
// 		GasLimit(thor.InitialGasLimit).
// 		Timestamp(launchTime).
// 		State(func(state *state.State) error {
// 			bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
// 			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
// 			builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(genesis.DevAccounts()[0].Address[:]))
// 			// for _, acc := range genesis.DevAccounts() {
// 			for _, acc := range accs {
// 				state.SetBalance(acc.addr, bal)
// 				state.SetEnergy(acc.addr, bal, launchTime)

// 				builtin.Authority.Native(state).Add(acc.addr, acc.addr, thor.Bytes32{}, acc.vrfpk.Bytes32())
// 			}
// 			return nil
// 		})

// 	stateCreator := state.NewCreator(db)
// 	genesisBlock, _, err := gen.Build(stateCreator)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	c, err := chain.New(db, genesisBlock)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	forkConfig := thor.ForkConfig{
// 		VIP191:    math.MaxUint32,
// 		ETH_CONST: math.MaxUint32,
// 		BLOCKLIST: 0,
// 	}
// 	con := New(c, stateCreator, forkConfig)

// 	return &testConsensus{
// 		t:            t,
// 		assert:       assert.New(t),
// 		con:          con,
// 		nodes:        accs,
// 		tag:          c.Tag(),
// 		chain:        c,
// 		stateCreator: stateCreator,
// 		genesisBlock: genesisBlock,
// 	}
// }

// // create a new block without committing to the state
// func (tc *testConsensus) newBlock(round uint32, txs []*tx.Transaction) {
// 	var (
// 		flow     *packer.Flow
// 		Proposer *account
// 		err      error
// 	)

// 	now := tc.con.Timestamp(round)
// 	parent := tc.chain.BestBlock()

// 	if now < parent.Header().Timestamp() {
// 		tc.t.Fatal("new block earlier than the best block")
// 	}

// 	// search for the legit Proposer
// 	for _, acc := range tc.Nodes {
// 		p := packer.New(tc.chain, tc.stateCreator, acc.addr, &acc.addr, thor.NoFork)
// 		flow, err = p.Schedule(parent.Header(), now)
// 		if err != nil {
// 			continue
// 		}

// 		if flow.When() == now {
// 			Proposer = acc
// 			break
// 		}
// 		flow = nil
// 	}
// 	if flow == nil {
// 		tc.t.Fatal("No Proposer found")
// 	}

// 	// add transactions
// 	for _, tx := range txs {
// 		flow.Adopt(tx)
// 	}

// 	// pack block summary
// 	bs, _, err := flow.PackTxSetAndBlockSummary(Proposer.Ethsk)
// 	if err != nil {
// 		tc.t.Fatal(err)
// 	}

// 	// pack endorsements
// 	for _, acc := range tc.Nodes {
// 		if ok, proof, _ := tc.con.IsCommittee(acc.vrfsk, now); ok {
// 			ed := block.NewEndorsement(bs, proof)
// 			sig, _ := crypto.Sign(ed.SigningHash().Bytes(), acc.Ethsk)
// 			ed = ed.WithSignature(sig)
// 			flow.AddEndoresement(ed)
// 		}
// 		if uint64(flow.NumOfEndorsements()) >= thor.CommitteeSize {
// 			break
// 		}
// 	}
// 	if uint64(flow.NumOfEndorsements()) < thor.CommitteeSize {
// 		tc.t.Errorf("Not enough endorsements added")
// 	}

// 	// pack block
// 	newBlock, stage, receipts, err := flow.Pack(Proposer.Ethsk)
// 	if err != nil {
// 		tc.t.Fatal(err)
// 	}

// 	// validate block
// 	if _, _, err := tc.con.Process(newBlock, flow.When()); err != nil {
// 		tc.t.Fatal(err)
// 	}

// 	tc.Parent = parent
// 	tc.Time = now
// 	tc.Original = newBlock
// 	tc.Proposer = Proposer
// 	tc.stage = stage
// 	tc.receipts = receipts
// }

// func (tc *testConsensus) commitNewBlock() {
// 	if _, err := tc.chain.GetBlockHeader(tc.Original.Header().ID()); err == nil {
// 		tc.t.Fatal("known in-chain block")
// 	}

// 	if _, err := tc.stage.Commit(); err != nil {
// 		tc.t.Fatal(err)
// 	}

// 	_, err := tc.chain.AddBlock(tc.Original, tc.receipts)
// 	if err != nil {
// 		tc.t.Fatal(err)
// 	}
// }

// func (tc *testConsensus) sign(blk *block.Block) *block.Block {
// 	sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), tc.Proposer.Ethsk)
// 	if err != nil {
// 		tc.t.Fatal(err)
// 	}
// 	return blk.WithSignature(sig)
// }

// /**
//  * rebuild takes the current block builder and re-compute the block summary
//  * and the endorsements. It then update the builder with the correct
//  * signatures and vrf proofs
//  */
// func (tc *testConsensus) rebuild(builder *block.Builder) *block.Builder {
// 	blk := builder.Build()
// 	header := blk.Header()

// 	// rebuild block summary
// 	bs := block.NewBlockSummary(
// 		header.ParentID(),
// 		header.TxsRoot(),
// 		header.Timestamp(),
// 		header.TotalScore())
// 	sig, err := crypto.Sign(bs.SigningHash().Bytes(), tc.Proposer.Ethsk)
// 	if err != nil {
// 		tc.t.Fatal(err)
// 	}
// 	bs = bs.WithSignature(sig)

// 	var (
// 		sigs   [][]byte
// 		proofs []*vrf.Proof
// 		N      = int(thor.CommitteeSize)
// 	)

// 	// rebuild endorsements
// 	for _, acc := range tc.Nodes {
// 		if ok, proof, err := tc.con.IsCommittee(acc.vrfsk, header.Timestamp()); ok {
// 			ed := block.NewEndorsement(bs, proof)
// 			sig, _ := crypto.Sign(ed.SigningHash().Bytes(), acc.Ethsk)
// 			proofs = append(proofs, proof)
// 			sigs = append(sigs, sig)
// 		} else if err != nil {
// 			tc.t.Fatal(err)
// 		}
// 		if len(proofs) >= N {
// 			break
// 		}
// 	}
// 	if len(sigs) != N {
// 		tc.t.Fatal("Not enough endorsements collected")
// 	}

// 	newBuilder := new(block.Builder).
// 		ParentID(header.ParentID()).
// 		Timestamp(header.Timestamp()).
// 		TotalScore(header.TotalScore()).
// 		GasLimit(header.GasLimit()).
// 		GasUsed(header.GasUsed()).
// 		Beneficiary(header.Beneficiary()).
// 		StateRoot(header.StateRoot()).
// 		ReceiptsRoot(header.ReceiptsRoot()).
// 		TransactionFeatures(header.TxsFeatures()).
// 		// update signatures and vrf proofs
// 		SigOnBlockSummary(sig).
// 		SigsOnEndorsement(sigs).
// 		VrfProofs(proofs)

// 	// add existing transactions
// 	for _, tx := range blk.Transactions() {
// 		newBuilder.Transaction(tx)
// 	}

// 	return newBuilder
// }

// func (tc *testConsensus) originalBuilder() *block.Builder {
// 	header := tc.Original.Header()
// 	return new(block.Builder).
// 		ParentID(header.ParentID()).
// 		Timestamp(header.Timestamp()).
// 		TotalScore(header.TotalScore()).
// 		GasLimit(header.GasLimit()).
// 		GasUsed(header.GasUsed()).
// 		Beneficiary(header.Beneficiary()).
// 		StateRoot(header.StateRoot()).
// 		ReceiptsRoot(header.ReceiptsRoot()).
// 		TransactionFeatures(header.TxsFeatures()).
// 		SigOnBlockSummary(header.SigOnBlockSummary()).
// 		SigsOnEndorsement(header.SigsOnEndoresment()).
// 		VrfProofs(header.VrfProofs())
// }

// func (tc *testConsensus) Consent(blk *block.Block) error {
// 	_, _, err := tc.con.Process(blk, tc.Time)
// 	return err
// }

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
	 * The test below is removed since the total score is used to reconstruct and validate
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
