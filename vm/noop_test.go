// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

func TestNoopCanTransfer(t *testing.T) {
	assert.True(t, NoopCanTransfer(nil, common.Address{}, big.NewInt(0)), "NoopCanTransfer should always return true")
}

func TestNoopTransfer(t *testing.T) {
	assert.NotPanics(t, func() { NoopTransfer(nil, common.Address{}, common.Address{}, big.NewInt(0)) }, "NoopTransfer should not panic")
}

func TestNoopEVMCallContext_Call(t *testing.T) {
	var ctx NoopEVMCallContext
	data, err := ctx.Call(nil, common.Address{}, nil, big.NewInt(0), big.NewInt(0))
	assert.Nil(t, err, "Call should not return an error")
	assert.Nil(t, data, "Call should return nil data")
}

func TestNoopEVMCallContext_CallCode(t *testing.T) {
	var ctx NoopEVMCallContext
	data, err := ctx.CallCode(nil, common.Address{}, nil, big.NewInt(0), big.NewInt(0))
	assert.Nil(t, err, "CallCode should not return an error")
	assert.Nil(t, data, "CallCode should return nil data")
}

func TestNoopEVMCallContext_Create(t *testing.T) {
	var ctx NoopEVMCallContext
	data, addr, err := ctx.Create(nil, nil, big.NewInt(0), big.NewInt(0))
	assert.Nil(t, err, "Create should not return an error")
	assert.Nil(t, data, "Create should return nil data")
	assert.Equal(t, common.Address{}, addr, "Create should return an empty address")
}

func TestNoopEVMCallContext_DelegateCall(t *testing.T) {
	var ctx NoopEVMCallContext
	data, err := ctx.DelegateCall(nil, common.Address{}, nil, big.NewInt(0))
	assert.Nil(t, err, "DelegateCall should not return an error")
	assert.Nil(t, data, "DelegateCall should return nil data")
}

func TestNoopStateDB_GetBalance(t *testing.T) {
	var db NoopStateDB
	balance := db.GetBalance(common.Address{})
	assert.Nil(t, balance, "GetBalance should return nil")
}

func TestNoopStateDB_CreateAccount(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.CreateAccount(common.Address{}) }, "CreateAccount should not panic")
}

func TestNoopStateDB_SubBalance(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.SubBalance(common.Address{}, big.NewInt(0)) }, "SubBalance should not panic")
}

func TestNoopStateDB_AddBalance(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.AddBalance(common.Address{}, big.NewInt(0)) }, "AddBalance should not panic")
}

func TestNoopStateDB_SetNonce(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.SetNonce(common.Address{}, 0) }, "SetNonce should not panic")
}

func TestNoopStateDB_GetNonce(t *testing.T) {
	var db NoopStateDB
	assert.Equal(t, db.GetNonce(common.Address{}), uint64(0))
}

func TestNoopStateDB_GetCodeSize(t *testing.T) {
	var db NoopStateDB
	assert.Equal(t, db.GetCodeSize(common.Address{}), 0)
}

func TestNoopStateDB_GetRefund(t *testing.T) {
	var db NoopStateDB
	assert.Equal(t, db.GetRefund(), uint64(0))
}

func TestNoopStateDB_GetCodeHash(t *testing.T) {
	var db NoopStateDB
	assert.Equal(t, db.GetCodeHash(common.Address{}), common.Hash{})
}

func TestNoopStateDB_GetCode(t *testing.T) {
	var db NoopStateDB
	assert.Nil(t, db.GetCode(common.Address{}))
}

func TestNoopStateDB_SetCode(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.SetCode(common.Address{}, []byte{}) }, "SetCode should not panic")
}

func TestNoopStateDB_AddRefund(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.AddRefund(0) }, "AddRefund should not panic")
}

func TestNoopStateDB_SetState(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.SetState(common.Address{}, common.Hash{}, common.Hash{}) }, "SetState should not panic")
}

func TestNoopStateDB_Suicide(t *testing.T) {
	var db NoopStateDB
	assert.False(t, db.Suicide(common.Address{}), "Suicide should return false")
}

func TestNoopStateDB_HasSuicided(t *testing.T) {
	var db NoopStateDB
	assert.False(t, db.HasSuicided(common.Address{}), "HasSuicided should return false")
}

func TestNoopStateDB_Exist(t *testing.T) {
	var db NoopStateDB
	assert.False(t, db.Exist(common.Address{}), "Exist should return false")
}

func TestNoopStateDB_Empty(t *testing.T) {
	var db NoopStateDB
	assert.False(t, db.Empty(common.Address{}), "Empty should return false")
}

func TestNoopStateDB_RevertToSnapshot(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.RevertToSnapshot(0) }, "RevertToSnapshot should not panic")
}

func TestNoopStateDB_Snapshot(t *testing.T) {
	var db NoopStateDB
	assert.Equal(t, 0, db.Snapshot(), "Snapshot should return 0")
}

func TestNoopStateDB_AddLog(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.AddLog(&types.Log{}) }, "AddLog should not panic")
}

func TestNoopStateDB_AddPreimage(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.AddPreimage(common.Hash{}, []byte{}) }, "AddPreimage should not panic")
}

func TestNoopStateDB_ForEachStorage(t *testing.T) {
	var db NoopStateDB
	assert.NotPanics(t, func() { db.ForEachStorage(common.Address{}, func(common.Hash, common.Hash) bool { return true }) }, "ForEachStorage should not panic")
}
