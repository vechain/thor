// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"bytes"
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

var (
	priv1 = string("dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65")
	priv2 = string("321d6443bc6177273b5abf54210fe806d451d6b7973bccc2384ef78bbcd0bf51")
)

func newBlock(parent *block.Header, quality uint32, score, ts uint64, pk *ecdsa.PrivateKey) *block.Block {
	bss := block.ComplexSignatures{}

	builder := new(block.Builder).
		ParentID(parent.ID()).
		TotalScore(score).
		Timestamp(ts).
		BackerSignatures(bss, quality)

	if quality != 0 {
		builder.Alpha([]byte{0x0})
	}

	b := builder.Build()

	sig, _ := crypto.Sign(b.Header().SigningHash().Bytes(), pk)

	return b.WithSignature(sig)
}

func diffID(parent *block.Header) (*block.Block, *block.Block) {
	signer, _ := crypto.HexToECDSA(priv1)

	b0 := newBlock(parent, 0, 0, 0, signer)
	b1 := newBlock(parent, 0, 0, 2, signer)

	c := bytes.Compare(b0.Header().ID().Bytes(), b1.Header().ID().Bytes())
	if c == 0 {
		panic("id should not be the same")
	}

	if c < 0 {
		return b1, b0
	}

	return b0, b1
}

func TestCompareChain(t *testing.T) {
	s1, _ := crypto.HexToECDSA(priv1)
	s2, _ := crypto.HexToECDSA(priv2)

	db := muxdb.NewMem()
	g := genesis.NewDevnet()
	b0, _, _, _ := g.Build(state.NewStater(db))

	repo, err := chain.NewRepository(db, b0)
	if err != nil {
		panic(err)
	}

	forkConfig := thor.NoFork
	forkConfig.VIP193 = 2

	b1 := newBlock(b0.Header(), 0, 10, 0, s1)
	b2 := newBlock(b1.Header(), 0, 10, 10, s2)
	largerID, lowerID := diffID(b0.Header())

	b3 := newBlock(b2.Header(), 1, 10, 30, s2)
	b4 := newBlock(b3.Header(), 1, 10, 40, s2)

	b3x := newBlock(b2.Header(), 1, 10, 20, s2)
	b4x := newBlock(b3x.Header(), 1, 10, 40, s2)

	receipts := tx.Receipts{}
	repo.AddBlock(b1, receipts)
	repo.AddBlock(b2, receipts)
	repo.AddBlock(b3, receipts)
	repo.AddBlock(b4, receipts)
	repo.AddBlock(b3x, receipts)
	repo.AddBlock(b4x, receipts)
	repo.AddBlock(largerID, receipts)
	repo.AddBlock(lowerID, receipts)

	node := &Node{
		repo:       repo,
		forkConfig: forkConfig,
	}

	tests := []struct {
		name string
		b1   *block.Block
		b2   *block.Block
		want bool
	}{
		{"higher score", newBlock(b0.Header(), 0, 11, 0, s2), b1, true},
		{"lower score", newBlock(b0.Header(), 0, 1, 0, s2), b1, false},
		{"equal score, larger id", largerID, lowerID, false},
		{"equal score, smaller id", lowerID, largerID, true},
		{"higher quality", newBlock(b1.Header(), 1, 10, 10, s1), b2, true},
		{"lower quality", newBlock(b2.Header(), 0, 10, 30, s2), b3, false},
		{"longer chain", b4, b3, true},
		{"shorter chain", b3, b4, false},
		{"equal quality,heavy in branch,same timestamp,higher score", newBlock(b2.Header(), 1, 11, 30, s2), b3, true},
		{"equal quality, no heavy in branch,higher score", newBlock(b3.Header(), 1, 11, 40, s2), b4, true},
		{"equal quality, no heavy in branch,lower score", b4, newBlock(b3.Header(), 1, 11, 40, s2), false},
		{"equal quality,heavy in branch,later timestamp", b4, b4x, true},
		{"equal quality,heavy in branch,earlier timestamp", b4x, b4, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo.AddBlock(tt.b1, tx.Receipts{})

			got, err := node.compare(tt.b1.Header(), tt.b2.Header())
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("n.compare() = %v, want %v", got, tt.want)
			}
		})
	}
}
