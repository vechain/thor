// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// params caches some dynamic params
type params struct {
	cache      *cache.PrioCache
	stater     *state.Stater
	forkConfig thor.ForkConfig
}

type entry struct {
	baseFee      *big.Int
	baseGasPrice *big.Int
}

func newParams(stater *state.Stater, forkConfig thor.ForkConfig) *params {
	return &params{
		cache:      cache.NewPrioCache(32),
		stater:     stater,
		forkConfig: forkConfig,
	}
}

// GetLegacyTxBaseGasPrice returns the legacy tx base gas price for the given block.
func (p *params) GetLegacyTxBaseGasPrice(head *chain.BlockSummary) (*big.Int, error) {
	var ent *entry
	if val, _, ok := p.cache.Get(head.Header.ID()); ok {
		ent = val.(*entry)
		if ent.baseGasPrice != nil {
			return ent.baseGasPrice, nil
		}
	} else {
		ent = &entry{}
	}

	baseGasPrice, err := builtin.Params.Native(p.stater.NewState(head.Root())).Get(thor.KeyLegacyTxBaseGasPrice)
	if err != nil {
		return nil, err
	}
	ent.baseGasPrice = baseGasPrice

	p.cache.Set(head.Header.ID(), ent, float64(head.Header.Number()))
	return ent.baseGasPrice, nil
}

// GetBaseFee returns the base fee for the given block.
// Before GALACTICA, the base fee is not set, so it returns nil.
func (p *params) GetBaseFee(head *chain.BlockSummary) *big.Int {
	if head.Header.Number()+1 < p.forkConfig.GALACTICA {
		return nil
	}

	var ent *entry
	if val, _, ok := p.cache.Get(head.Header.ID()); ok {
		ent = val.(*entry)
		if ent.baseFee != nil {
			return ent.baseFee
		}
	} else {
		ent = &entry{}
	}

	ent.baseFee = galactica.CalcBaseFee(head.Header, p.forkConfig)

	p.cache.Set(head.Header.ID(), ent, float64(head.Header.Number()))
	return ent.baseFee
}

func (p *params) GetFeatures(head *chain.BlockSummary) tx.Features {
	var feat tx.Features
	if head.Header.Number()+1 >= p.forkConfig.VIP191 {
		feat.SetDelegated(true)
	}
	return feat
}
