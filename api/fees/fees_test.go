package fees_test

import (
	"math/big"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/fees"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

var (
	tclient *thorclient.Client
)

func TestFees(t *testing.T) {
	ts := initFeesServer(t)
	defer ts.Close()

	tclient = thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T){
		"getFeeHistory": getFeeHistory,
	} {
		t.Run(name, tt)
	}
}

func initFeesServer(t *testing.T) *httptest.Server {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 1
	thorChain, err := testchain.NewIntegrationTestChainWithFork(forkConfig)
	require.NoError(t, err)

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	var dynFeeTx *tx.Transaction

	for i := 0; i < 9; i++ {
		dynFeeTx = tx.NewTxBuilder(tx.DynamicFeeTxType).
			ChainTag(thorChain.Repo().ChainTag()).
			MaxFeePerGas(big.NewInt(100000)).
			MaxPriorityFeePerGas(big.NewInt(100)).
			Expiration(10).
			Gas(21000).
			Nonce(uint64(i)).
			Clause(cla).
			BlockRef(tx.NewBlockRef(uint32(i))).
			MustBuild()
		dynFeeTx = tx.MustSign(dynFeeTx, genesis.DevAccounts()[0].PrivateKey)
		require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], dynFeeTx))
	}

	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)
	require.Len(t, allBlocks, 10)

	router := mux.NewRouter()
	fees.New(thorChain.Repo(), thorChain.Engine()).
		Mount(router, "/fees")

	return httptest.NewServer(router)
}

func getFeeHistory(t *testing.T) {
	// Test cases
}
