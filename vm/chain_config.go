// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/params"
)

// isForked returns whether a fork scheduled at block s is active at the given head block.
func isForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

// ChainConfig extends eth ChainConfig.
type ChainConfig struct {
	params.ChainConfig
	IstanbulBlock *big.Int `json:"istanbulBlock,omitempty"` // Istanbul switch block (nil = no fork, 0 = already on istanbul)
	ShanghaiBlock *big.Int `json:"shanghaiBlock,omitempty"` // Shanghai switch block (nil = no fork, 0 = already on shanghai)
	FusakaBlock   *big.Int `json:"fusakaBlock,omitempty"`   // Fusaka switch block (nil = no fork, 0 = already on fusaka)
}

// IsIstanbul returns whether num is either equal to the Istanbul fork block or greater.
func (c *ChainConfig) IsIstanbul(num *big.Int) bool {
	return isForked(c.IstanbulBlock, num)
}

// IsShanghai returns whether num is either equal to the Shanghai fork block or greater.
func (c *ChainConfig) IsShanghai(num *big.Int) bool {
	return isForked(c.ShanghaiBlock, num)
}

// IsFusaka returns whether num is either equal to the Fusaka fork block or greater.
// Fusaka is the VeChain INTERSTELLAR umbrella covering Dencun, Pectra and Osaka EVM changes.
func (c *ChainConfig) IsFusaka(num *big.Int) bool {
	return isForked(c.FusakaBlock, num)
}

// Rules wraps ChainConfig and is merely syntatic sugar or can be used for functions
// that do not have or require information about the block.
//
// Rules is a one time interface meaning that it shouldn't be used in between transition
// phases.
type Rules struct {
	ChainID                                   *big.Int
	IsHomestead, IsEIP150, IsEIP155, IsEIP158 bool
	IsByzantium                               bool
	IsIstanbul                                bool
	IsShanghai                                bool
	IsFusaka                                  bool
}

// Rules ensures c's ChainID is not nil.
func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainID := c.ChainID
	if chainID == nil {
		chainID = new(big.Int)
	}
	return Rules{
		ChainID:     new(big.Int).Set(chainID),
		IsHomestead: c.IsHomestead(num),
		IsEIP150:    c.IsEIP150(num),
		IsEIP155:    c.IsEIP155(num),
		IsEIP158:    c.IsEIP158(num),
		IsByzantium: c.IsByzantium(num),
		IsIstanbul:  c.IsIstanbul(num),
		IsShanghai:  c.IsShanghai(num),
		IsFusaka:    c.IsFusaka(num),
	}
}
