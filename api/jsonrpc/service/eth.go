// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package service

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Eth implements the eth_* JSON-RPC namespace.
type Eth struct{ b *Backend }

// NewEth creates an Eth service backed by b.
func NewEth(b *Backend) *Eth { return &Eth{b: b} }

// ChainId implements eth_chainId.
func (a *Eth) ChainId() (*hexutil.Big, error) { //nolint:revive // must be ChainId so it maps to eth_chainId
	return (*hexutil.Big)(new(big.Int).SetUint64(a.b.repo.ChainID())), nil
}

// BlockNumber implements eth_blockNumber.
func (a *Eth) BlockNumber() (hexutil.Uint64, error) {
	return hexutil.Uint64(a.b.repo.BestBlockSummary().Header.Number()), nil
}
