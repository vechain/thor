// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math/big"
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
	con        *Consensus
	time       uint64
	pk         *ecdsa.PrivateKey
	parent     *block.Block
	original   *block.Block
	forkConfig thor.ForkConfig
	tag        byte
}

func newTestConsensus() (*testConsensus, error) {
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
		return nil, err
	}

	repo, err := chain.NewRepository(db, parent)
	if err != nil {
		return nil, err
	}

	forkConfig := thor.NoFork
	forkConfig.BLOCKLIST = 0
	forkConfig.VIP214 = 2

	proposer := genesis.DevAccounts()[0]
	p := packer.New(repo, stater, proposer.Address, &proposer.Address, forkConfig)
	parentSum, _ := repo.GetBlockSummary(parent.Header().ID())
	flow, err := p.Schedule(parentSum, uint64(parent.Header().Timestamp()+100*thor.BlockInterval))
	if err != nil {
		return nil, err
	}

	b1, stage, receipts, err := flow.Pack(proposer.PrivateKey, 0, false)
	if err != nil {
		return nil, err
	}

	con := New(repo, stater, forkConfig)

	if _, _, err := con.Process(parentSum, b1, flow.When(), 0); err != nil {
		return nil, err
	}

	if _, err := stage.Commit(); err != nil {
		return nil, err
	}

	if err := repo.AddBlock(b1, receipts, 0); err != nil {
		return nil, err
	}

	if err := repo.SetBestBlockID(b1.Header().ID()); err != nil {
		return nil, err
	}

	proposer2 := genesis.DevAccounts()[1]
	p2 := packer.New(repo, stater, proposer2.Address, &proposer2.Address, forkConfig)
	b1sum, _ := repo.GetBlockSummary(b1.Header().ID())
	flow2, err := p2.Schedule(b1sum, uint64(b1.Header().Timestamp()+100*thor.BlockInterval))
	if err != nil {
		return nil, err
	}

	b2, _, _, err := flow2.Pack(proposer2.PrivateKey, 0, false)
	if err != nil {
		return nil, err
	}

	if _, _, err := con.Process(b1sum, b2, flow2.When(), 0); err != nil {
		return nil, err
	}

	return &testConsensus{
		con:        con,
		time:       flow2.When(),
		pk:         proposer.PrivateKey,
		parent:     b1,
		original:   b2,
		forkConfig: forkConfig,
		tag:        repo.ChainTag(),
	}, nil
}

func (tc *testConsensus) sign(builder *block.Builder) (*block.Block, error) {
	return tc.signWithKey(builder, tc.pk)
}

func (tc *testConsensus) signWithKey(builder *block.Builder, pk *ecdsa.PrivateKey) (*block.Block, error) {
	h := builder.Build().Header()

	if h.Number() >= tc.forkConfig.VIP214 {
		var alpha []byte
		if h.Number() == tc.forkConfig.VIP214 {
			alpha = tc.parent.Header().StateRoot().Bytes()
		} else {
			beta, err := tc.parent.Header().Beta()
			if err != nil {
				return nil, err
			}
			alpha = beta
		}
		_, proof, err := vrf.Prove(pk, alpha)
		if err != nil {
			return nil, err
		}

		blk := builder.Alpha(alpha).Build()

		ec, err := crypto.Sign(blk.Header().SigningHash().Bytes(), pk)
		if err != nil {
			return nil, err
		}

		sig, err := block.NewComplexSignature(ec, proof)
		if err != nil {
			return nil, err
		}
		return blk.WithSignature(sig), nil
	} else {
		blk := builder.Build()

		sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), pk)
		if err != nil {
			return nil, err
		}

		return blk.WithSignature(sig), nil
	}
}

func (tc *testConsensus) builder(header *block.Header) *block.Builder {
	return new(block.Builder).
		ParentID(header.ParentID()).
		Timestamp(header.Timestamp()).
		TotalScore(header.TotalScore()).
		GasLimit(header.GasLimit()).
		GasUsed(header.GasUsed()).
		Beneficiary(header.Beneficiary()).
		StateRoot(header.StateRoot()).
		ReceiptsRoot(header.ReceiptsRoot())
}

func (tc *testConsensus) consent(blk *block.Block) error {
	parentSum, err := tc.con.repo.GetBlockSummary(blk.Header().ParentID())
	if err != nil {
		return err
	}

	_, _, err = tc.con.Process(parentSum, blk, tc.time, 0)
	return err
}

func TestValidateBlockHeader(t *testing.T) {
	tc, err := newTestConsensus()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"ErrTimestampBehindParent", func(t *testing.T) {
				builder := tc.builder(tc.original.Header())

				blk, err := tc.sign(builder.Timestamp(tc.parent.Header().Timestamp()))
				if err != nil {
					t.Fatal(err)
				}

				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"block timestamp behind parents: parent %v, current %v",
						tc.parent.Header().Timestamp(),
						blk.Header().Timestamp(),
					),
				)
				assert.Equal(t, expected, err)

				blk, err = tc.sign(builder.Timestamp(tc.parent.Header().Timestamp() - 1))
				if err != nil {
					t.Fatal(err)
				}

				err = tc.consent(blk)
				expected = consensusError(
					fmt.Sprintf(
						"block timestamp behind parents: parent %v, current %v",
						tc.parent.Header().Timestamp(),
						blk.Header().Timestamp(),
					),
				)
				assert.Equal(t, expected, err)
			},
		},
		{
			"ErrInterval", func(t *testing.T) {
				builder := tc.builder(tc.original.Header())
				blk, err := tc.sign(builder.Timestamp(tc.original.Header().Timestamp() + 1))
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"block interval not rounded: parent %v, current %v",
						tc.parent.Header().Timestamp(),
						blk.Header().Timestamp(),
					),
				)
				assert.Equal(t, expected, err)
			},
		},
		{
			"ErrFutureBlock", func(t *testing.T) {
				builder := tc.builder(tc.original.Header())
				blk, err := tc.sign(builder.Timestamp(tc.time + thor.BlockInterval*2))
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := errFutureBlock
				assert.Equal(t, expected, err)
			},
		},
		{
			"InvalidGasLimit", func(t *testing.T) {
				builder := tc.builder(tc.original.Header())
				blk, err := tc.sign(builder.GasLimit(tc.parent.Header().GasLimit() * 2))
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"block gas limit invalid: parent %v, current %v",
						tc.parent.Header().GasLimit(),
						blk.Header().GasLimit(),
					),
				)
				assert.Equal(t, expected, err)

			},
		},
		{
			"ExceedGaUsed", func(t *testing.T) {
				builder := tc.builder(tc.original.Header())
				blk, err := tc.sign(builder.GasUsed(tc.original.Header().GasLimit() + 1))
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"block gas used exceeds limit: limit %v, used %v",
						tc.parent.Header().GasLimit(),
						blk.Header().GasUsed(),
					),
				)
				assert.Equal(t, expected, err)
			},
		},
		{
			"InvalidTotalScore", func(t *testing.T) {
				builder := tc.builder(tc.original.Header())
				blk, err := tc.sign(builder.TotalScore(tc.parent.Header().TotalScore()))
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"block total score invalid: parent %v, current %v",
						tc.parent.Header().TotalScore(),
						blk.Header().TotalScore(),
					),
				)
				assert.Equal(t, expected, err)
			},
		},
		{
			"InvalidBlockSignature", func(t *testing.T) {
				blk := tc.original.WithSignature(block.ComplexSignature(tc.original.Header().Signature()).Signature())

				err = tc.consent(blk)
				expected := consensusError("block signature length invalid: want 146 have 65")
				assert.Equal(t, expected, err)
			},
		},
		{
			"InvalidAlpha", func(t *testing.T) {
				var alpha [32]byte
				rand.Read(alpha[:])

				builder := tc.builder(tc.original.Header())
				blk := builder.Alpha(alpha[:]).Build().WithSignature(tc.original.Header().Signature())

				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"block alpha invalid: want %v, have %v",
						tc.parent.Header().StateRoot(),
						thor.Bytes32(alpha),
					),
				)
				assert.Equal(t, expected, err)
			},
		},
		{
			"Invalid VRF", func(t *testing.T) {
				var cs [146]byte
				rand.Read(cs[:])

				blk := tc.original.WithSignature(cs[:])

				_, theErr := blk.Header().Beta()

				err = tc.consent(blk)
				expected := consensusError(fmt.Sprintf("block VRF signature invalid: %v", theErr))
				assert.Equal(t, expected, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func TestVerifyBlock(t *testing.T) {
	tc, err := newTestConsensus()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"TxDepBroken", func(t *testing.T) {
				txID := txSign(txBuilder(tc.tag)).ID()
				tx := txSign(txBuilder(tc.tag).DependsOn(&txID))

				blk, err := tc.sign(tc.builder(tc.original.Header()).Transaction(tx))
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)

				expected := consensusError("tx dep broken")
				assert.Equal(t, expected, err)
			},
		},
		{
			"TxAlreadyExists", func(t *testing.T) {
				tx := txSign(txBuilder(tc.tag))
				blk, err := tc.sign(tc.builder(tc.original.Header()).Transaction(tx).Transaction(tx))
				if err != nil {
					t.Fatal(err)
				}

				err = tc.consent(blk)
				assert.Equal(t, err, consensusError("tx already exists"))
			},
		},
		{
			"GasUsedMismatch", func(t *testing.T) {
				blk, err := tc.sign(tc.builder(tc.original.Header()).GasUsed(100))
				if err != nil {
					t.Fatal(err)
				}
				expected := consensusError(
					fmt.Sprintf(
						"block gas used mismatch: want %v, have %v",
						blk.Header().GasUsed(),
						tc.original.Header().GasUsed(),
					),
				)
				err = tc.consent(blk)

				assert.Equal(t, err, expected)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func TestValidateBlockBody(t *testing.T) {
	tc, err := newTestConsensus()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"ErrTxsRootMismatch", func(t *testing.T) {
				transaction := txSign(txBuilder(tc.tag))
				transactions := tx.Transactions{transaction}
				blk := block.Compose(tc.original.Header(), transactions)
				expected := consensusError(
					fmt.Sprintf(
						"block txs root mismatch: want %v, have %v",
						tc.original.Header().TxsRoot(),
						transactions.RootHash(),
					),
				)
				err = tc.consent(blk)
				assert.Equal(t, expected, err)
			},
		},
		{
			"ErrChainTagMismatch", func(t *testing.T) {
				blk, err := tc.sign(tc.builder(tc.original.Header()).Transaction(txSign(txBuilder(tc.tag + 1))))
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"tx chain tag mismatch: want %v, have %v",
						tc.tag,
						tc.tag+1,
					),
				)
				assert.Equal(t, expected, err)
			},
		},
		{
			"ErrRefFutureBlock", func(t *testing.T) {
				blk, err := tc.sign(
					tc.builder(tc.original.Header()).Transaction(
						txSign(txBuilder(tc.tag).BlockRef(tx.NewBlockRef(100))),
					))
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError("tx ref future block: ref 100, current 2")
				assert.Equal(t, expected, err)
			},
		},
		{
			"TxOriginBlocked", func(t *testing.T) {
				thor.MockBlocklist([]string{genesis.DevAccounts()[9].Address.String()})
				tx := txBuilder(tc.tag).Build()
				sig, _ := crypto.Sign(tx.SigningHash().Bytes(), genesis.DevAccounts()[9].PrivateKey)
				tx = tx.WithSignature(sig)

				blk, err := tc.sign(
					tc.builder(tc.original.Header()).Transaction(tx),
				)
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf("tx origin blocked got packed: %v", genesis.DevAccounts()[9].Address),
				)
				assert.Equal(t, expected, err)
			},
		},
		{
			"TxSignerUnavailable", func(t *testing.T) {
				tx := txBuilder(tc.tag).Build()
				var sig [65]byte
				tx = tx.WithSignature(sig[:])

				_, theErr := tx.Origin()

				blk, err := tc.sign(
					tc.builder(tc.original.Header()).Transaction(tx),
				)
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError(fmt.Sprintf("tx signer unavailable: %v", theErr))
				assert.Equal(t, expected, err)
			},
		},
		{
			"UnsupportedFeatures", func(t *testing.T) {
				tx := txBuilder(tc.tag).Features(tx.Features(2)).Build()
				sig, _ := crypto.Sign(tx.SigningHash().Bytes(), genesis.DevAccounts()[2].PrivateKey)
				tx = tx.WithSignature(sig)

				blk, err := tc.sign(
					tc.builder(tc.original.Header()).Transaction(tx),
				)
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError("invalid tx: unsupported features")
				assert.Equal(t, expected, err)
			},
		},
		{
			"TxExpired", func(t *testing.T) {
				tx := txSign(txBuilder(tc.tag).BlockRef(tx.NewBlockRef(0)).Expiration(0))
				blk, err := tc.sign(tc.builder(tc.original.Header()).Transaction(tx).Transaction(tx))
				if err != nil {
					t.Fatal(err)
				}

				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"tx expired: ref %v, current %v, expiration %v",
						tx.BlockRef().Number(),
						tc.original.Header().Number(),
						tx.Expiration(),
					),
				)
				assert.Equal(t, err, expected)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}

func TestValidateProposer(t *testing.T) {
	tc, err := newTestConsensus()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"ErrInvalidSignature", func(t *testing.T) {
				blk := tc.builder(tc.parent.Header()).Build()

				err = tc.consent(blk)
				expected := consensusError("block signature length invalid: want 65 have 0")

				assert.Equal(t, expected, err)
			},
		},
		{
			"ErrSignerInvalid", func(t *testing.T) {
				pk, _ := crypto.GenerateKey()
				blk, err := tc.signWithKey(tc.builder(tc.original.Header()), pk)
				if err != nil {
					t.Fatal(err)
				}

				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"block signer invalid: %v unauthorized block proposer",
						thor.Address(crypto.PubkeyToAddress(pk.PublicKey)),
					),
				)
				assert.Equal(t, expected, err)
			},
		},
		{
			"ErrTimestampUnscheduled", func(t *testing.T) {
				blk, err := tc.signWithKey(tc.builder(tc.original.Header()), genesis.DevAccounts()[3].PrivateKey)
				if err != nil {
					t.Fatal(err)
				}

				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"block timestamp unscheduled: t %v, s %v",
						blk.Header().Timestamp(),
						thor.Address(crypto.PubkeyToAddress(genesis.DevAccounts()[3].PrivateKey.PublicKey)),
					),
				)
				assert.Equal(t, expected, err)
			},
		},
		{
			"TotalScoreInvalid", func(t *testing.T) {
				builder := tc.builder(tc.original.Header())
				blk, err := tc.sign(builder.TotalScore(tc.original.Header().TotalScore() + 100))
				if err != nil {
					t.Fatal(err)
				}
				err = tc.consent(blk)
				expected := consensusError(
					fmt.Sprintf(
						"block total score invalid: want %v, have %v",
						tc.original.Header().TotalScore(),
						tc.original.Header().TotalScore()+100,
					),
				)

				assert.Equal(t, expected, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t)
		})
	}
}
