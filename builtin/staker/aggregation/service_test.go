// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package aggregation

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker/stakes"
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
	assert.Equal(t, big.NewInt(0), agg.LockedVET)
	assert.Equal(t, big.NewInt(0), agg.PendingVET)
	assert.Equal(t, big.NewInt(0), agg.ExitingVET)
}

func TestService_AddAndSub_Pending(t *testing.T) {
	svc, _, _ := newSvc()

	v := thor.BytesToAddress([]byte("v"))
	ws := stakes.NewWeightedStake(big.NewInt(1000), 200)

	assert.NoError(t, svc.AddPendingVET(v, ws))

	agg, err := svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1000), agg.PendingVET)
	assert.Equal(t, big.NewInt(2000), agg.PendingWeight)

	err = svc.SubPendingVet(v, ws)
	assert.NoError(t, err)

	agg, err = svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), agg.PendingVET)
	assert.Equal(t, big.NewInt(0), agg.PendingWeight)
}

func TestService_Renew(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))

	wsAdd := stakes.NewWeightedStake(big.NewInt(3000), 200)
	assert.NoError(t, svc.AddPendingVET(v, wsAdd))

	renew1, _, err := svc.Renew(v)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(3000), renew1.NewLockedVET)
	assert.Equal(t, big.NewInt(6000), renew1.NewLockedWeight)

	assert.NoError(t, svc.SignalExit(v, stakes.NewWeightedStake(big.NewInt(1000), 200), renew1.NewLockedVET))

	assert.NoError(t, svc.AddPendingVET(v, stakes.NewWeightedStake(big.NewInt(500), 200)))

	renew2, _, err := svc.Renew(v)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(-500), renew2.NewLockedVET)
	assert.Equal(t, big.NewInt(-1000), renew2.NewLockedWeight)
	assert.Equal(t, big.NewInt(500), renew2.QueuedDecrease)
	assert.Equal(t, big.NewInt(1000), renew2.QueuedDecreaseWeight)

	agg, err := svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2500), agg.LockedVET)    // 3000 + 500
	assert.Equal(t, big.NewInt(5000), agg.LockedWeight) // 6000 + 1000
	assert.Equal(t, big.NewInt(0), agg.PendingVET)
	assert.Equal(t, big.NewInt(0), agg.ExitingVET)
}

func TestService_Exit(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))

	assert.NoError(t, svc.AddPendingVET(v, stakes.NewWeightedStake(big.NewInt(2000), 200)))
	_, _, err := svc.Renew(v)
	assert.NoError(t, err)

	assert.NoError(t, svc.AddPendingVET(v, stakes.NewWeightedStake(big.NewInt(800), 200)))

	exit, err := svc.Exit(v)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2000), exit.ExitedTVL)
	assert.Equal(t, big.NewInt(4000), exit.ExitedTVLWeight)
	assert.Equal(t, big.NewInt(800), exit.QueuedDecrease)
	assert.Equal(t, big.NewInt(1600), exit.QueuedDecreaseWeight)

	agg, err := svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.True(t, agg.IsEmpty())
}

func TestService_SignalExit(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))

	ws := stakes.NewWeightedStake(big.NewInt(1500), 200) // weight 3000
	assert.NoError(t, svc.SignalExit(v, ws, ws.VET()))

	agg, err := svc.GetAggregation(v)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1500), agg.ExitingVET)
	assert.Equal(t, big.NewInt(4500), agg.ExitingWeight)
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

	err := svc.AddPendingVET(v, stakes.NewWeightedStake(big.NewInt(1000), 100))
	assert.ErrorContains(t, err, "failed to get validator aggregation")
}

func TestService_SubPendingVet_ErrorOnGet(t *testing.T) {
	svc, contract, st := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	poisonMapping(st, contract, v)

	err := svc.SubPendingVet(v, stakes.NewWeightedStake(big.NewInt(1000), 100))
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

	err := svc.SignalExit(v, stakes.NewWeightedStake(big.NewInt(1000), 100), big.NewInt(0))
	assert.ErrorContains(t, err, "failed to get validator aggregation")
}
