// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestConsensus(t *testing.T) {
	obValue := reflect.ValueOf(newTestConsensus(t))
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

func txSign(builder *tx.Builder) *tx.Transaction {
	transaction := builder.Build()
	sig, _ := crypto.Sign(transaction.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	return transaction.WithSignature(sig)
}

type testConsensus struct {
	t          *testing.T
	assert     *assert.Assertions
	con        *Consensus
	time       uint64
	pk         *ecdsa.PrivateKey
	parent     *block.Block
	original   *block.Block
	tag        byte
	revertedID thor.Bytes32
}

func newTestConsensus(t *testing.T) *testConsensus {
	db := muxdb.NewMem()

	launchTime := uint64(1526400000)
	gen := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(launchTime).
		State(func(state *state.State) error {
			bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(genesis.DevAccounts()[0].Address[:]))
			for _, acc := range genesis.DevAccounts() {
				state.SetBalance(acc.Address, bal)
				state.SetEnergy(acc.Address, bal, launchTime)
				builtin.Authority.Native(state).Add(acc.Address, acc.Address, thor.Bytes32{})
			}
			return nil
		})

	stater := state.NewStater(db)
	parent, _, _, err := gen.Build(stater)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := chain.NewRepository(db, parent)
	if err != nil {
		t.Fatal(err)
	}

	forkConfig := thor.NoFork
	forkConfig.BLOCKLIST = 0
	forkConfig.VIP191 = 0
	forkConfig.VIP193 = 0

	proposer := genesis.DevAccounts()[0]
	backer := genesis.DevAccounts()[1]

	//b1---------
	randomPriv, _ := crypto.HexToECDSA("c2d81000421244612975e457bf2a57b3c488565e93a1b52994bda322e20cfea7")
	tx := txBuilder(repo.ChainTag()).Build()
	sig, _ := crypto.Sign(tx.SigningHash().Bytes(), randomPriv)
	tx = tx.WithSignature(sig)

	p := packer.New(repo, stater, proposer.Address, &proposer.Address, forkConfig)
	flow, err := p.Schedule(parent, parent.Header().Timestamp()+thor.BlockInterval)
	if err != nil {
		t.Fatal(err)
	}
	_ = flow.Adopt(tx)

	var (
		proof [81]byte
		beta  [32]byte
	)
	rand.Read(proof[:])
	rand.Read(beta[:])

	proposal, _ := flow.Draft(proposer.PrivateKey)
	hash := proposal.Hash()
	backerSig, _ := crypto.Sign(hash.Bytes(), backer.PrivateKey)
	bs, _ := block.NewComplexSignature(proof[:], backerSig)

	flow.AddBackerSignature(bs, beta[:], backer.Address)
	b1, stage, receipts, err := flow.Pack(proposer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stage.Commit()
	if err != nil {
		t.Fatal(err)
	}
	err = repo.AddBlock(b1, receipts)
	if err != nil {
		t.Fatal(err)
	}
	//-----------

	p = packer.New(repo, stater, proposer.Address, &proposer.Address, forkConfig)
	flow, err = p.Schedule(b1, b1.Header().Timestamp()+thor.BlockInterval)
	if err != nil {
		t.Fatal(err)
	}
	original, _, _, err := flow.Pack(proposer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	con := New(repo, stater, forkConfig)

	if _, _, _, err := con.Process(original, flow.When()); err != nil {
		t.Fatal(err)
	}

	return &testConsensus{
		t:          t,
		assert:     assert.New(t),
		con:        con,
		time:       flow.When(),
		pk:         proposer.PrivateKey,
		parent:     b1,
		original:   original,
		tag:        repo.ChainTag(),
		revertedID: tx.ID(),
	}
}

func (tc *testConsensus) sign(blk *block.Block, pk *ecdsa.PrivateKey) *block.Block {
	alpha := blk.Header().Alpha()

	sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), pk)
	if err != nil {
		tc.t.Fatal(err)
	}

	_, proof, err := ecvrf.NewSecp256k1Sha256Tai().Prove(pk, alpha)
	if err != nil {
		tc.t.Fatal(err)
	}

	cs, _ := block.NewComplexSignature(proof, sig)
	return blk.WithSignature(cs)
}

func (tc *testConsensus) originalBuilder() *block.Builder {
	var features tx.Features
	features |= tx.DelegationFeature

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
		Alpha(header.Alpha()).
		BackerSignatures(block.ComplexSignatures{}, tc.parent.Header().TotalQuality()).
		TransactionFeatures(features)
}

func (tc *testConsensus) consent(blk *block.Block) error {
	_, _, _, err := tc.con.Process(blk, tc.time)
	return err
}

func (tc *testConsensus) TestValidateBlockHeader() {
	triggers := make(map[string]func())
	triggers["triggerErrTimestampBehindParent"] = func() {
		build := tc.originalBuilder()

		blk := tc.sign(build.Timestamp(tc.parent.Header().Timestamp()).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block timestamp behind parents: parent %v, current %v",
				tc.parent.Header().Timestamp(),
				blk.Header().Timestamp(),
			),
		)
		tc.assert.Equal(expect, err)

		blk = tc.sign(build.Timestamp(tc.parent.Header().Timestamp()-1).Build(), tc.pk)
		err = tc.consent(blk)
		expect = consensusError(
			fmt.Sprintf(
				"block timestamp behind parents: parent %v, current %v",
				tc.parent.Header().Timestamp(),
				blk.Header().Timestamp(),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrInterval"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.Timestamp(tc.original.Header().Timestamp()+1).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block interval not rounded: parent %v, current %v",
				tc.parent.Header().Timestamp(),
				blk.Header().Timestamp(),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrFutureBlock"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.Timestamp(tc.time+thor.BlockInterval*2).Build(), tc.pk)
		err := tc.consent(blk)
		tc.assert.Equal(errFutureBlock, err)
	}
	triggers["triggerInvalidGasLimit"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.GasLimit(tc.parent.Header().GasLimit()*2).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block gas limit invalid: parent %v, current %v",
				tc.parent.Header().GasLimit(),
				blk.Header().GasLimit(),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerExceedGaUsed"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.GasUsed(tc.original.Header().GasLimit()+1).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block gas used exceeds limit: limit %v, used %v",
				tc.parent.Header().GasLimit(),
				blk.Header().GasUsed(),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerInvalidTotalScore"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.TotalScore(tc.parent.Header().TotalScore()).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block total score invalid: parent %v, current %v",
				tc.parent.Header().TotalScore(),
				blk.Header().TotalScore(),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerInvalidTxFeatures"] = func() {
		build := tc.originalBuilder()
		var features, originFeatures tx.Features
		originFeatures |= tx.DelegationFeature
		features |= 2
		blk := tc.sign(build.TransactionFeatures(features).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block txs features invalid: want %v, have %v",
				originFeatures,
				features,
			),
		)
		tc.assert.Equal(expect, err)
	}

	for _, trigger := range triggers {
		trigger()
	}
}

func (tc *testConsensus) TestKnownBlock() {
	err := tc.consent(tc.parent)
	tc.assert.Equal(errKnownBlock, err)
}

func (tc *testConsensus) TestParentMissing() {
	build := tc.originalBuilder()
	blk := tc.sign(build.ParentID(tc.original.Header().ID()).Build(), tc.pk)
	err := tc.consent(blk)
	tc.assert.Equal(errParentMissing, err)
}

func (tc *testConsensus) TestValidateBlockBody() {
	triggers := make(map[string]func())
	triggers["triggerErrTxSignerUnavailable"] = func() {
		blk := tc.sign(tc.originalBuilder().Transaction(txBuilder(tc.tag).Build()).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError("tx signer unavailable: invalid signature length")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrTxsRootMismatch"] = func() {
		transaction := txSign(txBuilder(tc.tag))
		transactions := tx.Transactions{transaction}
		blk := tc.sign(block.Compose(tc.original.Header(), transactions, nil), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block txs root mismatch: want %v, have %v",
				tc.original.Header().TxsRoot(),
				transactions.RootHash(),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrGasUsed"] = func() {
		blk := tc.sign(tc.originalBuilder().GasUsed(100).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError("block gas used mismatch: want 100, have 0")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrReceiptRootMismatch"] = func() {
		blk := tc.sign(tc.originalBuilder().ReceiptsRoot(thor.Bytes32{}).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block receipts root mismatch: want %v, have %v",
				thor.Bytes32{},
				tc.original.Header().ReceiptsRoot(),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrStateRootMismatch"] = func() {
		blk := tc.sign(tc.originalBuilder().StateRoot(thor.Bytes32{}).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block state root mismatch: want %v, have 0xe049292984c1036f3098a3b4c44bb66d9fc1457725fae84db4609071aeab635e",
				thor.Bytes32{},
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrChainTagMismatch"] = func() {
		err := tc.consent(
			tc.sign(
				tc.originalBuilder().Transaction(
					txSign(txBuilder(tc.tag+1)),
				).Build(),
				tc.pk,
			),
		)
		expect := consensusError(
			fmt.Sprintf(
				"tx chain tag mismatch: want %v, have %v",
				tc.tag,
				tc.tag+1,
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrRefFutureBlock"] = func() {
		err := tc.consent(
			tc.sign(
				tc.originalBuilder().Transaction(
					txSign(txBuilder(tc.tag).BlockRef(tx.NewBlockRef(100))),
				).Build(),
				tc.pk,
			),
		)
		expect := consensusError("tx ref future block: ref 100, current 2")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerTxOriginBlocked"] = func() {
		thor.MockBlocklist([]string{genesis.DevAccounts()[9].Address.String()})
		t := txBuilder(tc.tag).Build()
		sig, _ := crypto.Sign(t.SigningHash().Bytes(), genesis.DevAccounts()[9].PrivateKey)
		t = t.WithSignature(sig)

		blk := tc.sign(
			tc.originalBuilder().Transaction(t).Build(),
			tc.pk,
		)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf("tx origin blocked got packed: %v", genesis.DevAccounts()[9].Address),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerTxExpired"] = func() {
		err := tc.consent(
			tc.sign(
				tc.originalBuilder().Transaction(
					txSign(txBuilder(tc.tag).BlockRef(tx.NewBlockRef(0)).Expiration(1)),
				).Build(),
				tc.pk,
			),
		)
		expect := consensusError("tx expired: ref 0, current 2, expiration 1")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerInvalidFeatures"] = func() {
		err := tc.consent(
			tc.sign(
				tc.originalBuilder().Transaction(
					txSign(txBuilder(tc.tag).Features(2)),
				).Build(),
				tc.pk,
			),
		)
		expect := consensusError("invalid tx: unsupported features")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerTxDepBroken"] = func() {
		txID := txSign(txBuilder(tc.tag)).ID()
		tx := txSign(txBuilder(tc.tag).DependsOn(&txID))
		err := tc.consent(
			tc.sign(
				tc.originalBuilder().Transaction(tx).Build(),
				tc.pk,
			),
		)
		tc.assert.Equal(consensusError("tx dep broken"), err)
	}
	triggers["triggerTxReverted"] = func() {
		tx := txSign(txBuilder(tc.tag).DependsOn(&tc.revertedID))
		err := tc.consent(
			tc.sign(
				tc.originalBuilder().Transaction(tx).Build(),
				tc.pk,
			),
		)
		tc.assert.Equal(consensusError("tx dep reverted"), err)
	}
	triggers["triggerTxAlreadyExists"] = func() {
		tx := txSign(txBuilder(tc.tag))
		err := tc.consent(
			tc.sign(
				tc.originalBuilder().Transaction(tx).Transaction(tx).Build(),
				tc.pk,
			),
		)
		tc.assert.Equal(consensusError("tx already exists"), err)
	}
	triggers["triggerInvalidBackersRootCount"] = func() {
		b := tc.sign(tc.originalBuilder().BackerSignatures(block.ComplexSignatures{}, tc.parent.Header().TotalQuality()).Build(), tc.pk)

		var (
			proof [81]byte
			sig   [65]byte
		)
		rand.Read(proof[:])
		rand.Read(sig[:])

		bs, _ := block.NewComplexSignature(proof[:], sig[:])

		as := block.ComplexSignatures{bs}
		blk := block.Compose(b.Header(), tx.Transactions{}, as)
		err := tc.consent(blk)
		expect := consensusError(fmt.Sprintf("block backers root mismatch: want %v, have %v", b.Header().BackerSignaturesRoot(), as.RootHash()))
		tc.assert.Equal(expect, err)
	}
	triggers["triggerBackersNotInPower"] = func() {
		header := tc.original.Header()
		priv, _ := crypto.GenerateKey()
		hash := block.NewProposal(header.ParentID(), header.TxsRoot(), header.GasLimit(), header.Timestamp()).Hash()

		alpha := tc.original.Header().Alpha()
		_, proof, _ := ecvrf.NewSecp256k1Sha256Tai().Prove(priv, alpha)
		backerSig, _ := crypto.Sign(hash.Bytes(), priv)
		bs, _ := block.NewComplexSignature(proof[:], backerSig)

		blk := tc.sign(tc.originalBuilder().BackerSignatures(block.ComplexSignatures{bs}, tc.parent.Header().TotalQuality()).Build(), tc.pk)

		err := tc.consent(blk)
		expect := consensusError(fmt.Sprintf("backer: %v is not an authority", thor.Address(crypto.PubkeyToAddress(priv.PublicKey))))
		tc.assert.Equal(expect, err)
	}
	triggers["triggerLeaderCannotBeBacker"] = func() {
		header := tc.original.Header()
		proposer := genesis.DevAccounts()[0]
		hash := block.NewProposal(header.ParentID(), header.TxsRoot(), header.GasLimit(), header.Timestamp()).Hash()

		alpha := tc.original.Header().Alpha()
		_, proof, _ := ecvrf.NewSecp256k1Sha256Tai().Prove(proposer.PrivateKey, alpha)
		backerSig, _ := crypto.Sign(hash.Bytes(), proposer.PrivateKey)
		bs, _ := block.NewComplexSignature(proof[:], backerSig)

		blk := tc.sign(tc.originalBuilder().BackerSignatures(block.ComplexSignatures{bs}, tc.parent.Header().TotalQuality()).Build(), tc.pk)

		err := tc.consent(blk)
		expect := consensusError("block signer cannot back itself")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerInvalidProof"] = func() {
		header := tc.original.Header()
		backer := genesis.DevAccounts()[1]

		hash := block.NewProposal(header.ParentID(), header.TxsRoot(), header.GasLimit(), header.Timestamp()).Hash()
		alpha := tc.original.Header().Alpha()
		_, proof, _ := ecvrf.NewSecp256k1Sha256Tai().Prove(backer.PrivateKey, alpha)

		backerSig, _ := crypto.Sign(hash.Bytes(), backer.PrivateKey)
		bs, _ := block.NewComplexSignature(proof, backerSig)
		blk := tc.sign(tc.originalBuilder().BackerSignatures(block.ComplexSignatures{bs}, tc.parent.Header().TotalQuality()).Build(), tc.pk)

		err := tc.consent(blk)
		expect := consensusError(fmt.Sprintf("invalid proof from %v", backer.Address))
		tc.assert.Equal(expect, err)
	}
	triggers["triggerNotSorted"] = func() {
		header := tc.original.Header()

		initialSize := thor.CommitteMemberSize
		thor.MockCommitteMemberSize(thor.InitialMaxBlockProposers)
		hash := block.NewProposal(header.ParentID(), header.TxsRoot(), header.GasLimit(), header.Timestamp()).Hash()
		alpha := tc.original.Header().Alpha()

		_, proof1, _ := ecvrf.NewSecp256k1Sha256Tai().Prove(genesis.DevAccounts()[1].PrivateKey, alpha)
		sig1, _ := crypto.Sign(hash.Bytes(), genesis.DevAccounts()[1].PrivateKey)
		bs1, _ := block.NewComplexSignature(proof1, sig1)

		_, proof2, _ := ecvrf.NewSecp256k1Sha256Tai().Prove(genesis.DevAccounts()[2].PrivateKey, alpha)
		sig2, _ := crypto.Sign(hash.Bytes(), genesis.DevAccounts()[2].PrivateKey)
		bs2, _ := block.NewComplexSignature(proof2, sig2)

		blk := tc.sign(tc.originalBuilder().BackerSignatures(block.ComplexSignatures{bs1, bs2}, tc.parent.Header().TotalQuality()).Build(), tc.pk)

		err := tc.consent(blk)
		expect := consensusError("backer signatures are not in ascending order(by beta)")
		tc.assert.Equal(expect, err)
		thor.MockCommitteMemberSize(initialSize)
	}
	triggers["triggerInvalidAlpha"] = func() {
		var alpha [32 + 4]byte
		rand.Read(alpha[:])

		err := tc.consent(tc.sign(tc.originalBuilder().Alpha(alpha[:]).Build(), tc.pk))
		expect := consensusError(fmt.Sprintf("alpha mismatch: want %s, have %s", hexutil.Bytes(tc.original.Header().Alpha()), hexutil.Bytes(alpha[:])))
		tc.assert.Equal(expect, err)
	}
	triggers["triggerInvalidSignerProof"] = func() {
		var proof [81]byte
		rand.Read(proof[:])

		cs, _ := block.NewComplexSignature(proof[:], block.ComplexSignature(tc.original.Header().Signature()).Signature())
		err := tc.consent(tc.original.WithSignature(cs))

		tc.assert.Contains(err.Error(), "failed to verify VRF in header:")
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
		expect := consensusError("invalid signature length")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrSignerInvalid"] = func() {
		pk, _ := crypto.GenerateKey()
		err := tc.consent(tc.sign(tc.originalBuilder().Build(), pk))
		expect := consensusError(
			fmt.Sprintf(
				"block signer invalid: %v unauthorized or inactive block proposer",
				thor.Address(crypto.PubkeyToAddress(pk.PublicKey)),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrTimestampUnscheduled"] = func() {
		blk := tc.sign(tc.originalBuilder().Build(), genesis.DevAccounts()[3].PrivateKey)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block timestamp unscheduled: t %v, s %v",
				blk.Header().Timestamp(),
				thor.Address(crypto.PubkeyToAddress(genesis.DevAccounts()[3].PrivateKey.PublicKey)),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerTotalScoreInvalid"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.TotalScore(tc.original.Header().TotalScore()+100).Build(), tc.pk)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block total score invalid: want %v, have %v",
				blk.Header().TotalScore()-100,
				blk.Header().TotalScore(),
			))
		tc.assert.Equal(expect, err)
	}

	for _, trigger := range triggers {
		trigger()
	}
}
