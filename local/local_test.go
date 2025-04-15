// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package local

import (
	"log/slog"
	"math/big"
	"testing"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

func sendAddValidatorTx(t *testing.T, acc genesis.DevAccount) {
	method, ok := builtin.Staker.ABI.MethodByName("addValidator")
	if !ok {
		t.Fatal("method not found")
	}
	data, err := method.EncodeInput(acc.Address, uint32(2), true)
	if err != nil {
		t.Fatal(err)
	}
	stake := big.NewInt(25_000_000)
	stake = stake.Mul(stake, big.NewInt(1e18))
	clause := tx.NewClause(&builtin.Staker.Address).WithValue(stake).WithData(data)

	sendClause(t, clause, acc)
}

func Test_AddValidator_1(t *testing.T) {
	t.Skip()
	sendAddValidatorTx(t, genesis.DevAccounts()[2])
}

func Test_AddValidator_2(t *testing.T) {
	t.Skip()
	sendAddValidatorTx(t, genesis.DevAccounts()[1])
}

func sendClause(t *testing.T, clause *tx.Clause, acc genesis.DevAccount) {
	client := thorclient.New("http://localhost:8669")
	chainTag, err := client.ChainTag()
	if err != nil {
		t.Fatal(err)
	}
	transaction := new(tx.Builder).
		Clause(clause).
		ChainTag(chainTag).
		Gas(1_000_000).
		BlockRef(tx.NewBlockRef(0)).
		Expiration(1_000_000).
		Nonce(datagen.RandUint64()).
		GasPriceCoef(255).
		Build()
	transaction = tx.MustSign(transaction, acc.PrivateKey)
	res, err := client.SendTransaction(transaction)
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("sent tx", "from", acc.Address, "txid", res.ID)
}
