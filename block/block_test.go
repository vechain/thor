// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vrf"
)

func TestBlockProof(t *testing.T) {
	tx1 := tx.NewBuilder(tx.TypeLegacy).Clause(tx.NewClause(&thor.Address{})).Clause(tx.NewClause(&thor.Address{})).Build()
	tx2 := tx.NewBuilder(tx.TypeDynamicFee).Clause(tx.NewClause(nil)).Build()

	privKey := string("dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65")
	alpha := thor.MustParseBytes32("0x68abc4fe6b911dd388eac9252513071dd4edea83e183c4b477dc65dd59359c2c")

	var (
		emptyRoot   = thor.BytesToBytes32([]byte("0"))
		beneficiary = thor.BytesToAddress([]byte("abc"))
	)

	blk := new(Builder).
		GasUsed(1000).
		Transaction(tx1).
		Transaction(tx2).
		GasLimit(14000).
		TotalScore(101).
		StateRoot(emptyRoot).
		ReceiptsRoot(emptyRoot).
		Timestamp(1761554386318816000).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		ParentID(emptyRoot).
		Alpha(alpha.Bytes()).
		Beneficiary(beneficiary).
		Build()

	key, _ := crypto.HexToECDSA(privKey)
	ec, err := crypto.Sign(blk.Header().SigningHash().Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}

	_, proof, err := vrf.Prove(key, alpha.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	sig, err := NewComplexSignature(ec, proof)
	if err != nil {
		t.Fatal(err)
	}
	blk = blk.WithSignature(sig)

	_, err = blk.Header().Beta()
	assert.Nil(t, err)

	newProof := make([]byte, len(proof))

	copy(newProof, proof[0:33])
	sig, err = NewComplexSignature(ec, newProof)
	if err != nil {
		t.Fatal(err)
	}
	blk = blk.WithSignature(sig)

	_, err = blk.Header().Beta()
	assert.ErrorContains(t, err, "invalid proof: value c is zero")

	copy(newProof, proof[0:47])
	sig, err = NewComplexSignature(ec, newProof)
	if err != nil {
		t.Fatal(err)
	}
	blk = blk.WithSignature(sig)

	_, err = blk.Header().Beta()
	assert.ErrorContains(t, err, "invalid proof: value s is zero")
}
