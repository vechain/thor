// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math"
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

	forkConfig := thor.ForkConfig{
		VIP191:    0,
		ETH_CONST: math.MaxUint32,
		BLOCKLIST: 0,
		VIP193:    0,
	}

	proposer := genesis.DevAccounts()[0]
	backer := genesis.DevAccounts()[1]

	//b0---------
	randomPriv, _ := crypto.HexToECDSA("c2d81000421244612975e457bf2a57b3c488565e93a1b52994bda322e20cfea7")
	tx := txBuilder(repo.ChainTag()).Build()
	sig, _ := crypto.Sign(tx.SigningHash().Bytes(), randomPriv)
	tx = tx.WithSignature(sig)

	p := packer.New(repo, stater, proposer.Address, &proposer.Address, forkConfig)
	flow, err := p.Schedule(parent.Header(), 1591709330)
	if err != nil {
		t.Fatal(err)
	}
	_ = flow.Adopt(tx)
	dec, _ := flow.Declare(proposer.PrivateKey)
	alpha := dec.Alpha(proposer.Address)
	_, proof, _ := ecvrf.NewSecp256k1Sha256Tai().Prove(backer.PrivateKey, alpha.Bytes())
	pub := crypto.CompressPubkey(&backer.PrivateKey.PublicKey)
	flow.AddBackerSignature(block.NewBackerSignature(pub, proof))
	b0, stage, receipts, err := flow.Pack(proposer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stage.Commit()
	if err != nil {
		t.Fatal(err)
	}
	err = repo.AddBlock(b0, receipts)
	if err != nil {
		t.Fatal(err)
	}
	//-----------

	p = packer.New(repo, stater, proposer.Address, &proposer.Address, forkConfig)
	flow, err = p.Schedule(b0.Header(), uint64(1591709330))
	if err != nil {
		t.Fatal(err)
	}

	original, _, _, err := flow.Pack(proposer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	con := New(repo, stater, forkConfig)

	if _, _, err := con.Process(original, flow.When()); err != nil {
		t.Fatal(err)
	}

	return &testConsensus{
		t:          t,
		assert:     assert.New(t),
		con:        con,
		time:       flow.When(),
		pk:         proposer.PrivateKey,
		parent:     b0,
		original:   original,
		tag:        repo.ChainTag(),
		revertedID: tx.ID(),
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
		BackerSignatures(block.BackerSignatures{}, 1).
		TransactionFeatures(features)
}

func (tc *testConsensus) consent(blk *block.Block) error {
	_, _, err := tc.con.Process(blk, tc.time)
	return err
}

func (tc *testConsensus) TestValidateBlockHeader() {
	triggers := make(map[string]func())
	triggers["triggerErrTimestampBehindParent"] = func() {
		build := tc.originalBuilder()

		blk := tc.sign(build.Timestamp(tc.parent.Header().Timestamp()).Build())
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block timestamp behind parents: parent %v, current %v",
				tc.parent.Header().Timestamp(),
				blk.Header().Timestamp(),
			),
		)
		tc.assert.Equal(expect, err)

		blk = tc.sign(build.Timestamp(tc.parent.Header().Timestamp() - 1).Build())
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
		blk := tc.sign(build.Timestamp(tc.original.Header().Timestamp() + 1).Build())
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
		blk := tc.sign(build.Timestamp(tc.time + thor.BlockInterval*2).Build())
		err := tc.consent(blk)
		tc.assert.Equal(errFutureBlock, err)
	}
	triggers["triggerInvalidGasLimit"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.GasLimit(tc.parent.Header().GasLimit() * 2).Build())
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
		blk := tc.sign(build.GasUsed(tc.original.Header().GasLimit() + 1).Build())
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
		blk := tc.sign(build.TotalScore(tc.parent.Header().TotalScore()).Build())
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
		blk := tc.sign(build.TransactionFeatures(features).Build())
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
	triggers["triggerInvalidBackersCount"] = func() {
		blk := tc.sign(tc.originalBuilder().BackerSignatures(block.BackerSignatures{}, 0).Build())
		err := tc.consent(blk)
		expect := consensusError("block total backers count invalid: parent 1, current 0")
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
	blk := tc.sign(build.ParentID(tc.original.Header().ID()).Build())
	err := tc.consent(blk)
	tc.assert.Equal(errParentMissing, err)
}

func (tc *testConsensus) TestValidateBlockBody() {
	triggers := make(map[string]func())
	triggers["triggerErrTxSignerUnavailable"] = func() {
		blk := tc.sign(tc.originalBuilder().Transaction(txBuilder(tc.tag).Build()).Build())
		err := tc.consent(blk)
		expect := consensusError("tx signer unavailable: invalid signature length")
		tc.assert.Equal(expect, err)
	}

	triggers["triggerErrTxsRootMismatch"] = func() {
		transaction := txSign(txBuilder(tc.tag))
		transactions := tx.Transactions{transaction}
		blk := tc.sign(block.Compose(tc.original.Header(), transactions, nil))
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
		blk := tc.sign(tc.originalBuilder().GasUsed(100).Build())
		err := tc.consent(blk)
		expect := consensusError("block gas used mismatch: want 100, have 0")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrReceiptRootMismatch"] = func() {
		blk := tc.sign(tc.originalBuilder().ReceiptsRoot(thor.Bytes32{}).Build())
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
		blk := tc.sign(tc.originalBuilder().StateRoot(thor.Bytes32{}).Build())
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
					txSign(txBuilder(tc.tag + 1)),
				).Build(),
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
			),
		)
		tc.assert.Equal(consensusError("tx dep broken"), err)
	}
	triggers["triggerTxReverted"] = func() {
		tx := txSign(txBuilder(tc.tag).DependsOn(&tc.revertedID))
		err := tc.consent(
			tc.sign(
				tc.originalBuilder().Transaction(tx).Build(),
			),
		)
		tc.assert.Equal(consensusError("tx dep reverted"), err)
	}
	triggers["triggerTxAlreadyExists"] = func() {
		tx := txSign(txBuilder(tc.tag))
		err := tc.consent(
			tc.sign(
				tc.originalBuilder().Transaction(tx).Transaction(tx).Build(),
			),
		)
		tc.assert.Equal(consensusError("tx already exists"), err)
	}
	triggers["triggerInvalidBackersCount"] = func() {
		blk := tc.sign(tc.originalBuilder().BackerSignatures(block.BackerSignatures{}, 2).Build())
		err := tc.consent(blk)
		expect := consensusError("block total backers count invalid: want 2, have 1")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerInvalidBackersRootCount"] = func() {
		b := tc.sign(tc.originalBuilder().BackerSignatures(block.BackerSignatures{}, 2).Build())

		var (
			pub   [33]byte
			proof [81]byte
		)
		rand.Read(pub[:])
		rand.Read(proof[:])

		as := block.BackerSignatures{block.NewBackerSignature(pub[:], proof[:])}
		blk := block.Compose(b.Header(), tx.Transactions{}, as)
		err := tc.consent(blk)
		expect := consensusError(fmt.Sprintf("block backers root mismatch: want %v, have %v", b.Header().BackerSignaturesRoot(), as.RootHash()))
		tc.assert.Equal(expect, err)
	}
	triggers["triggerBackersNotInPower"] = func() {
		var proof [81]byte
		rand.Read(proof[:])

		priv, _ := crypto.GenerateKey()
		pub := crypto.CompressPubkey(&priv.PublicKey)

		as := block.BackerSignatures{block.NewBackerSignature(pub[:], proof[:])}

		blk := tc.sign(tc.originalBuilder().BackerSignatures(as, tc.parent.Header().TotalBackersCount()).Build())

		err := tc.consent(blk)
		expect := consensusError(fmt.Sprintf("backer: %v is not an authority", thor.Address(crypto.PubkeyToAddress(priv.PublicKey))))
		tc.assert.Equal(expect, err)
	}
	triggers["triggerLeaderCannotBeBacker"] = func() {
		var proof [81]byte
		rand.Read(proof[:])
		pub := crypto.CompressPubkey(&genesis.DevAccounts()[0].PrivateKey.PublicKey)
		bs := block.BackerSignatures{block.NewBackerSignature(pub[:], proof[:])}

		blk := tc.sign(tc.originalBuilder().BackerSignatures(bs, tc.parent.Header().TotalBackersCount()).Build())

		err := tc.consent(blk)
		expect := consensusError("block signer cannot back itself")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerBackerNotLucky"] = func() {
		proof := hexutil.MustDecode("0x03b002a7366b1b4c02cddd595554645d15709a408146fd9bc7c1304c853395f0c8544c6101eb470c669dd02b1fb9757c51ac3788de0c3ac3ea284f2f1ba3e1ff8da8a6f65a7fb588273ddf1fbc9f1dd705")
		backer := genesis.DevAccounts()[1]

		pub := crypto.CompressPubkey(&backer.PrivateKey.PublicKey)
		bs := block.NewBackerSignature(pub, proof)
		blk := tc.sign(tc.originalBuilder().BackerSignatures(block.BackerSignatures{bs}, tc.parent.Header().TotalBackersCount()).Build())

		err := tc.consent(blk)
		expect := consensusError("0xd3ae78222beadb038203be21ed5ce7c9b1bff602's proof is not lucky enough to be a backer")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerNotSorted"] = func() {
		p1 := hexutil.MustDecode("0x02fc5e1ffe4256cd9b0bae8eca9132c7657ea19652d9f79f73bc5e13a3eec59b0f754d89004ceece0c65af73e7a469dee7a1e97222ad8109a60e76bc51a3da907fbe9eaf270766b29422409612d5fa85bb")
		b1 := crypto.CompressPubkey(&genesis.DevAccounts()[1].PrivateKey.PublicKey)
		bs1 := block.NewBackerSignature(b1, p1)

		p2 := hexutil.MustDecode("0x0216fae1949bb9bca7c6645230c4217a4ca7cc4f8a0e459974acfe02284261c9ae84ff16d5085a2eca503dc41fcf0cc4a4cf21374c57511485848045371ca32ebcf384d395b194a45481107ecd68fd1826")
		b2 := crypto.CompressPubkey(&genesis.DevAccounts()[6].PrivateKey.PublicKey)
		bs2 := block.NewBackerSignature(b2, p2)

		blk := tc.sign(tc.originalBuilder().GasLimit(10000043).BackerSignatures(block.BackerSignatures{bs1, bs2}, tc.parent.Header().TotalBackersCount()).Build())

		err := tc.consent(blk)
		expect := consensusError("backer signatures are not in ascending order(by beta)")
		tc.assert.Equal(expect, err)
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
		expect := consensusError("block signer unavailable: invalid signature length")
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrSignerInvalid"] = func() {
		blk := tc.originalBuilder().Build()
		pk, _ := crypto.GenerateKey()
		sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), pk)
		blk = blk.WithSignature(sig)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block signer invalid: %v unauthorized block proposer",
				thor.Address(crypto.PubkeyToAddress(pk.PublicKey)),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerErrTimestampUnscheduled"] = func() {
		blk := tc.originalBuilder().Build()
		sig, _ := crypto.Sign(blk.Header().SigningHash().Bytes(), genesis.DevAccounts()[1].PrivateKey)
		blk = blk.WithSignature(sig)
		err := tc.consent(blk)
		expect := consensusError(
			fmt.Sprintf(
				"block timestamp unscheduled: t %v, s %v",
				blk.Header().Timestamp(),
				thor.Address(crypto.PubkeyToAddress(genesis.DevAccounts()[1].PrivateKey.PublicKey)),
			),
		)
		tc.assert.Equal(expect, err)
	}
	triggers["triggerTotalScoreInvalid"] = func() {
		build := tc.originalBuilder()
		blk := tc.sign(build.TotalScore(tc.original.Header().TotalScore() + 100).Build())
		err := tc.consent(blk)
		expect := consensusError("block total score invalid: want 2, have 102")
		tc.assert.Equal(expect, err)
	}

	for _, trigger := range triggers {
		trigger()
	}
}
