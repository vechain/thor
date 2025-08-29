// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package aggregation

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/types"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func newSvc() (*Service, thor.Address, *state.State) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("agg"))
	svc := New(solidity.NewContext(addr, st, nil))
	return svc, addr, st
}

func poisonMapping(st *state.State, contract thor.Address, validator thor.Address) {
	slot := thor.Blake2b(validator.Bytes(), slotAggregations.Bytes())
	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})
}

func TestService_GetAggregation_ZeroInit(t *testing.T) {
	svc, _, _ := newSvc()

	v := thor.BytesToAddress([]byte("validator"))
	agg, err := svc.GetAggregation(v)
	assert.NoError(t, err)

	assert.True(t, agg.IsEmpty())
	assert.Equal(t, uint64(0), agg.LockedVET)
	assert.Equal(t, uint64(0), agg.PendingVET)
	assert.Equal(t, uint64(0), agg.ExitingVET)
}

func TestService_AddAndSub_Pending(t *testing.T) {
	svc, _, _ := newSvc()

	v := thor.BytesToAddress([]byte("v"))
	ws := types.NewWeightedStakeWithMultiplier(1000, 200)

	assert.NoError(t, svc.AddPendingVET(v, ws))

	agg, err := svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1000), agg.PendingVET)
	assert.Equal(t, uint64(2000), agg.PendingWeight)

	err = svc.SubPendingVet(v, ws)
	assert.NoError(t, err)

	agg, err = svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), agg.PendingVET)
	assert.Equal(t, uint64(0), agg.PendingWeight)
}

func TestService_Renew(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))

	wsAdd := types.NewWeightedStakeWithMultiplier(3000, 200)
	assert.NoError(t, svc.AddPendingVET(v, wsAdd))

	renew1, _, err := svc.Renew(v)
	assert.NoError(t, err)
	assert.Equal(t, uint64(3000), renew1.LockedIncrease.VET)
	assert.Equal(t, uint64(6000), renew1.LockedIncrease.Weight)

	assert.NoError(t, svc.SignalExit(v, types.NewWeightedStakeWithMultiplier(uint64(1000), 200)))

	assert.NoError(t, svc.AddPendingVET(v, types.NewWeightedStakeWithMultiplier(500, 200)))

	renew2, _, err := svc.Renew(v)
	assert.NoError(t, err)

	assert.Equal(t, uint64(500), renew2.LockedIncrease.VET)
	assert.Equal(t, uint64(1000), renew2.LockedIncrease.Weight)
	assert.Equal(t, uint64(1000), renew2.LockedDecrease.VET)
	assert.Equal(t, uint64(2000), renew2.LockedDecrease.Weight)
	assert.Equal(t, uint64(500), renew2.QueuedDecrease.VET)
	assert.Equal(t, uint64(1000), renew2.QueuedDecrease.Weight)

	agg, err := svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2500), agg.LockedVET)    // 3000 + 500
	assert.Equal(t, uint64(5000), agg.LockedWeight) // 6000 + 1000
	assert.Equal(t, uint64(0), agg.PendingVET)
	assert.Equal(t, uint64(0), agg.ExitingVET)
}

func TestService_Exit(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))

	assert.NoError(t, svc.AddPendingVET(v, types.NewWeightedStakeWithMultiplier(uint64(2000), 200)))
	_, _, err := svc.Renew(v)
	assert.NoError(t, err)

	assert.NoError(t, svc.AddPendingVET(v, types.NewWeightedStakeWithMultiplier(uint64(800), 200)))

	exit, err := svc.Exit(v)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2000), exit.ExitedTVL.VET)
	assert.Equal(t, uint64(4000), exit.ExitedTVL.Weight)
	assert.Equal(t, uint64(800), exit.QueuedDecrease.VET)
	assert.Equal(t, uint64(1600), exit.QueuedDecrease.Weight)

	agg, err := svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.True(t, agg.IsEmpty())
}

func TestService_SignalExit(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))

	ws := types.NewWeightedStakeWithMultiplier(uint64(1500), 200) // weight 3000
	assert.NoError(t, svc.SignalExit(v, ws))

	agg, err := svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1500), agg.ExitingVET)
	assert.Equal(t, uint64(3000), agg.ExitingWeight)
}

func TestService_GetAggregation_Error(t *testing.T) {
	svc, contract, st := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	poisonMapping(st, contract, v)

	_, err := svc.GetAggregation(v)
	assert.ErrorContains(t, err, "failed to get validator aggregation")
}

func TestService_AddPendingVET_ErrorOnGet(t *testing.T) {
	svc, contract, st := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	poisonMapping(st, contract, v)

	err := svc.AddPendingVET(v, types.NewWeightedStakeWithMultiplier(uint64(1000), 100))
	assert.ErrorContains(t, err, "failed to get validator aggregation")
}

func TestService_SubPendingVet_ErrorOnGet(t *testing.T) {
	svc, contract, st := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	poisonMapping(st, contract, v)

	err := svc.SubPendingVet(v, types.NewWeightedStakeWithMultiplier(uint64(1000), 100))
	assert.ErrorContains(t, err, "failed to get validator aggregation")
}

func TestService_Renew_ErrorOnGet(t *testing.T) {
	svc, contract, st := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	poisonMapping(st, contract, v)

	_, _, err := svc.Renew(v)
	assert.ErrorContains(t, err, "failed to get validator aggregation")
}

func TestService_Exit_ErrorOnGet(t *testing.T) {
	svc, contract, st := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	poisonMapping(st, contract, v)

	_, err := svc.Exit(v)
	assert.ErrorContains(t, err, "failed to get validator aggregation")
}

func TestService_SignalExit_ErrorOnGet(t *testing.T) {
	svc, contract, st := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	poisonMapping(st, contract, v)

	err := svc.SignalExit(v, types.NewWeightedStakeWithMultiplier(uint64(1000), 100))
	assert.ErrorContains(t, err, "failed to get validator aggregation")
}
