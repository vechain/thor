// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"math/big"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/poa"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestAuthority_Hayabusa_TransitionPeriod(t *testing.T) {
	config := thor.SoloFork
	config.HAYABUSA = 2

	chain, err := testchain.NewWithFork(config)
	assert.NoError(t, err)

	consensus := New(chain.Repo(), chain.Stater(), config)

	// mint block 1: PoA - update the MBP
	blk, _, _ := mintMbpBlock(t, chain, 1)

	endorsorBal, err := getEndorsorBalance(blk.Header, chain)
	assert.NoError(t, err)

	// mint block 2: chain should set the staker contract, still using PoA
	best, parent, st := mintBlock(t, chain)
	handler, err := consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)
	proposers, err := getBestCandidates(chain, consensus, handler)
	assert.NoError(t, err)
	assert.Len(t, proposers, 1)

	// mint block 3: validator moves their staker to the contract
	best, parent, st = mintAddValidatorBlock(t, chain)
	handler, err = consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)
	proposers, err = getBestCandidates(chain, consensus, handler)
	assert.NoError(t, err)
	assert.Len(t, proposers, 1)

	// check the endorsor balance has reduced
	newEndorsorBal, err := getEndorsorBalance(best.Header, chain)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Add(newEndorsorBal, minStake).Cmp(endorsorBal), 0)

	// check the staker contract has the correct stake
	masterStake, err := getMasterStake(chain, blk.Header)
	assert.NoError(t, err)
	assert.Equal(t, masterStake.Stake.Cmp(minStake), 0)
}

func getBestCandidates(chain *testchain.Chain, con *Consensus, handler cacheHandler) ([]poa.Proposer, error) {
	receipts := make([]*tx.Receipt, 0)
	blk, err := chain.BestBlock()
	if err != nil {
		return nil, err
	}
	for _, transaction := range blk.Transactions() {
		receipt, err := chain.GetTxReceipt(transaction.ID())
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	if err := handler(receipts); err != nil {
		return nil, err
	}

	var candidates *poa.Candidates
	if entry, ok := con.authorityCache.Get(blk.Header().ID()); ok {
		candidates = entry.(*poa.Candidates).Copy()
	} else {
		return nil, errors.New("candidates not found")
	}
	st := chain.Stater().NewState(chain.Repo().BestBlockSummary().Root())
	signer, err := blk.Header().Signer()
	if err != nil {
		return nil, err
	}

	proposers, err := candidates.Pick(st, con.authorityBalanceCheck(blk.Header(), st, signer))
	if err != nil {
		return nil, err
	}
	return proposers, nil
}

func getEndorsorBalance(blk *block.Header, chain *testchain.Chain) (*big.Int, error) {
	st := chain.Stater().NewState(chain.Repo().BestBlockSummary().Root())
	signer, err := blk.Signer()
	if err != nil {
		return nil, err
	}
	_, endorsor, _, _, _ := builtin.Authority.Native(st).Get(signer)
	balance, err := st.GetBalance(endorsor)
	if err != nil {
		return nil, err
	}
	return balance, nil
}

func getMasterStake(chain *testchain.Chain, blk *block.Header) (*staker.Validator, error) {
	st := chain.Stater().NewState(chain.Repo().BestBlockSummary().Root())
	signer, err := blk.Signer()
	if err != nil {
		return nil, err
	}
	return builtin.Staker.Native(st).Get(signer)
}
