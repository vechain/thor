package e2e

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/tests/e2e/client"
	"github.com/vechain/thor/v2/tests/e2e/network"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAccountBalance(t *testing.T) {
	network.StartCompose(t)

	account, err := client.Default.GetAccount("0xf077b491b355E64048cE21E3A6Fc4751eEeA77fa")

	assert.NoError(t, err, "GetAccount()")

	balance, err := account.Balance.MarshalText()

	assert.NoError(t, err, "MarshalText()")

	assert.Equal(t, hexutil.Encode(balance), "0x307831346164663462373332303333346239303030303030")
}
