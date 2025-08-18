// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package delegation

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func poisonDelegationSlot(st *state.State, contract thor.Address, delegationID *big.Int) {
	slot := thor.Blake2b(delegationID.Bytes(), slotDelegations.Bytes())
	st.SetRawStorage(contract, slot, rlp.RawValue{0xFF})
}

func poisonCounterGet(st *state.State, contract thor.Address) {
	st.SetRawStorage(contract, slotDelegationsCounter, rlp.RawValue{0xFF})
}

func newSvc() (*Service, thor.Address, *state.State) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("deleg"))
	svc := New(solidity.NewContext(addr, st, nil))
	return svc, addr, st
}

func TestService_Add_And_GetDelegation(t *testing.T) {
	svc, _, _ := newSvc()

	id, err := svc.Add(thor.BytesToAddress([]byte("v")), 2, big.NewInt(1000), 50)
	assert.NoError(t, err)
	assert.NotNil(t, id)

	del, err := svc.GetDelegation(id)
	assert.NoError(t, err)
	assert.Equal(t, thor.BytesToAddress([]byte("v")), del.Validation)
	assert.Equal(t, uint32(2), del.FirstIteration)
	assert.Equal(t, big.NewInt(1000), del.Stake)
	assert.Equal(t, uint8(50), del.Multiplier)
	assert.Nil(t, del.LastIteration)
}

func TestService_Add_InputValidation(t *testing.T) {
	svc, _, _ := newSvc()

	_, err := svc.Add(thor.Address{}, 1, big.NewInt(0), 10)
	assert.ErrorContains(t, err, "stake must be greater than 0")

	_, err = svc.Add(thor.Address{}, 1, big.NewInt(1), 0)
	assert.ErrorContains(t, err, "multiplier cannot be 0")
}

func TestService_SetDelegation_RoundTrip(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	id, err := svc.Add(v, 1, big.NewInt(100), 25)
	assert.NoError(t, err)

	del, err := svc.GetDelegation(id)
	assert.NoError(t, err)
	del.Multiplier = 99
	del.FirstIteration = 5

	assert.NoError(t, svc.SetDelegation(id, del, false))

	got, err := svc.GetDelegation(id)
	assert.NoError(t, err)
	assert.Equal(t, uint8(99), got.Multiplier)
	assert.Equal(t, uint32(5), got.FirstIteration)
}

func TestService_SignalExit(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	id, err := svc.Add(v, 3, big.NewInt(1000), 10)
	assert.NoError(t, err)

	del, err := svc.GetDelegation(id)
	assert.NoError(t, err)

	assert.NoError(t, svc.SignalExit(del, id, 7))

	del2, err := svc.GetDelegation(id)
	assert.NoError(t, err)
	assert.NotNil(t, del2.LastIteration)
	assert.Equal(t, uint32(7), *del2.LastIteration)

	assert.ErrorContains(t, svc.SignalExit(del2, id, 8), "already disabled")
}

func TestService_SignalExit_NotActive(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	id, err := svc.Add(v, 1, big.NewInt(100), 10)
	assert.NoError(t, err)

	del, err := svc.GetDelegation(id)
	assert.NoError(t, err)

	del.Stake = big.NewInt(0)
	assert.ErrorContains(t, svc.SignalExit(del, id, 5), "delegation is not active")
}

func TestService_Withdraw(t *testing.T) {
	svc, _, _ := newSvc()
	v := thor.BytesToAddress([]byte("v"))
	id, err := svc.Add(v, 1, big.NewInt(12345), 10)
	assert.NoError(t, err)

	del, err := svc.GetDelegation(id)
	assert.NoError(t, err)

	withdraw, err := svc.Withdraw(del, id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(12345), withdraw)

	after, err := svc.GetDelegation(id)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), after.Stake)
}

func TestService_GetDelegation_NotFoundZeroValue(t *testing.T) {
	svc, _, _ := newSvc()
	id := big.NewInt(999)
	del, err := svc.GetDelegation(id)
	assert.NoError(t, err)
	assert.NotNil(t, del)
}

func TestService_GetDelegation_Error(t *testing.T) {
	svc, addr, st := newSvc()
	id := big.NewInt(1)

	poisonDelegationSlot(st, addr, id)

	_, err := svc.GetDelegation(id)
	assert.ErrorContains(t, err, "failed to get delegation")
	assert.ErrorContains(t, err, "state: rlp:")
}

func TestService_Add_CounterGetError(t *testing.T) {
	svc, contract, st := newSvc()
	poisonCounterGet(st, contract)

	_, err := svc.Add(thor.BytesToAddress([]byte("v")), 1, big.NewInt(10), 1)
	assert.Error(t, err)
}

func TestService_Add_CounterSetOverflow(t *testing.T) {
	svc, contract, st := newSvc()
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	st.SetStorage(contract, slotDelegationsCounter, thor.BytesToBytes32(max.Bytes()))

	_, err := svc.Add(thor.BytesToAddress([]byte("v")), 1, big.NewInt(10), 1)
	assert.ErrorContains(t, err, "failed to increment delegation ID counter")
	assert.ErrorContains(t, err, "uint256 overflow")
}
