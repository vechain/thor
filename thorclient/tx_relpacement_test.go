package thorclient_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

func waitForNext(t *testing.T, client *thorclient.Client) {
	best, err := client.Block("best")
	require.NoError(t, err)

	// wait for next block
	for {
		next, _ := client.Block(fmt.Sprintf("%d", best.Number+1))
		if next != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestTxReplacement(t *testing.T) {
	t.Skip()
	client := thorclient.New("http://localhost:8669")
	chainTag, err := client.ChainTag()
	require.NoError(t, err)
	to := thor.MustParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")

	clause := tx.NewClause(&to).WithValue(big.NewInt(1000))
	fees, err := client.FeesHistory(1, "next")
	require.NoError(t, err)

	baseFee := fees.BaseFees[0].ToInt()
	baseFee = baseFee.Mul(baseFee, big.NewInt(120))
	baseFee = baseFee.Div(baseFee, big.NewInt(100))

	lowFeeTx := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(chainTag).
		Clause(clause).
		Gas(1_000_000).
		MaxFeePerGas(baseFee).
		Expiration(100000).
		Nonce(datagen.RandUint64()).
		Replacement(1000).
		Build()
	lowFeeTx = tx.MustSign(lowFeeTx, genesis.DevAccounts()[0].PrivateKey)

	highFee := fees.BaseFees[0].ToInt()
	highFee = highFee.Mul(highFee, big.NewInt(150))
	highFee = highFee.Div(highFee, big.NewInt(100))

	highFeeTx := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(chainTag).
		Clause(clause).
		Gas(1_000_000).
		MaxFeePerGas(highFee).
		Expiration(100000).
		Nonce(datagen.RandUint64()).
		Replacement(1000).
		Build()
	highFeeTx = tx.MustSign(highFeeTx, genesis.DevAccounts()[0].PrivateKey)

	// wait for next block
	t.Log("txs built and signed, waiting for next block")
	waitForNext(t,client)

	lowFeeRes, err := client.SendTransaction(lowFeeTx)
	require.NoError(t, err)

	highFeeRes, err := client.SendTransaction(highFeeTx)
	require.NoError(t, err)

	t.Logf("txs sent, waiting for next block (low=%s, high=%s)", lowFeeRes.ID.String(), highFeeRes.ID.String())
	waitForNext(t, client)

	lowFeeTxRes, err := client.Transaction(lowFeeRes.ID)
	require.NoError(t, err)
	require.Nil(t, lowFeeTxRes.Meta)

	highFeeTxRes, err := client.Transaction(highFeeRes.ID)
	require.NoError(t, err)
	t.Logf("tx id %s", highFeeTxRes.Meta.BlockID)
}
