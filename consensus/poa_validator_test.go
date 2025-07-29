// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/test/testchain"
)

func TestAuthority_Hayabusa_TransitionPeriod(t *testing.T) {
	setup := newHayabusaSetup(t)

	// mint block 1: PoA - update the MBP
	blk, _, _ := setup.mintMbpBlock(1)

	endorsorBal, err := getEndorsorBalance(blk.Header, setup.chain)
	assert.NoError(t, err)

	// mint block 2: chain should set the staker contract, still using PoA
	best, parent, st := setup.mintBlock()
	_, err = setup.consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)

	// mint block 3: validator moves their stake to the contract
	best, parent, st = setup.mintAddValidatorBlock()
	_, err = setup.consensus.validateAuthorityProposer(best.Header, parent.Header, st)
	assert.NoError(t, err)

	// check the endorsor balance has reduced
	newEndorsorBal, err := getEndorsorBalance(best.Header, setup.chain)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Add(newEndorsorBal, minStake).Cmp(endorsorBal), 0)

	// check the staker contract has the correct stake
	masterStake, err := getMasterStake(setup.chain, blk.Header)
	assert.NoError(t, err)
	assert.Equal(t, masterStake.PendingLocked.Cmp(minStake), 0)
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

func getMasterStake(chain *testchain.Chain, blk *block.Header) (*validation.Validation, error) {
	st := chain.Stater().NewState(chain.Repo().BestBlockSummary().Root())
	signer, err := blk.Signer()
	if err != nil {
		return nil, err
	}
	staker := builtin.Staker.Native(st)
	validator, err := staker.Get(signer)
	return validator, err
}
