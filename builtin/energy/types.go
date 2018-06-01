// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
)

type (
	initialSupply struct {
		Token     *big.Int
		Energy    *big.Int
		BlockTime uint64
	}
	totalAddSub struct {
		TotalAdd *big.Int
		TotalSub *big.Int
	}
)

var (
	_ state.StorageDecoder = (*initialSupply)(nil)
	_ state.StorageEncoder = (*initialSupply)(nil)

	_ state.StorageDecoder = (*totalAddSub)(nil)
	_ state.StorageEncoder = (*totalAddSub)(nil)
)

// Encode implements state.StorageEncoder.
func (i *initialSupply) Encode() ([]byte, error) {
	if i.Token.Sign() == 0 && i.Energy.Sign() == 0 && i.BlockTime == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(i)
}

// Decode implements state.StorageDecoder.
func (i *initialSupply) Decode(data []byte) error {
	if len(data) == 0 {
		*i = initialSupply{
			&big.Int{},
			&big.Int{},
			0,
		}
		return nil
	}
	return rlp.DecodeBytes(data, i)
}

// Encode implements state.StorageEncoder.
func (t *totalAddSub) Encode() ([]byte, error) {
	if t.TotalAdd.Sign() == 0 && t.TotalSub.Sign() == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(t)
}

// Decode implements state.StorageDecoder.
func (t *totalAddSub) Decode(data []byte) error {
	if len(data) == 0 {
		*t = totalAddSub{
			&big.Int{},
			&big.Int{},
		}
		return nil
	}
	return rlp.DecodeBytes(data, t)
}
