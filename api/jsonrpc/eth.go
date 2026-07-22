// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/thor"
)

type ethAPI struct{ b *backend }

// eth_chainId
func (a *ethAPI) ChainId() (*hexutil.Big, error) {
	return (*hexutil.Big)(new(big.Int).SetUint64(a.b.repo.ChainID())), nil
}

// eth_blockNumber
func (a *ethAPI) BlockNumber() (hexutil.Uint64, error) {
	return hexutil.Uint64(a.b.repo.BestBlockSummary().Header.Number()), nil
}

// eth_getBalance. Bootstrap: blockParam empty/"latest" => best; other values are
// forwarded verbatim to ParseRevision. Full BlockNumberOrHash union + ethereum<->thor
// revision mapping is deferred to Phase 1.
func (a *ethAPI) GetBalance(ctx context.Context, addr common.Address, blockParam *string) (*hexutil.Big, error) {
	rev := "best"
	if blockParam != nil && *blockParam != "" && *blockParam != "latest" {
		rev = *blockParam
	}
	_, st, err := a.b.stateForRevision(rev)
	if err != nil {
		return nil, &jsonError{Code: errcodeDefault, Message: err.Error()}
	}
	bal, err := st.GetBalance(thor.Address(addr))
	if err != nil {
		return nil, &jsonError{Code: errcodeDefault, Message: err.Error()}
	}
	return (*hexutil.Big)(bal), nil
}
