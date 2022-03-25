// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package prototype_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin/prototype"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func M(a ...interface{}) []interface{} {
	return a
}

func TestPrototype(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, thor.Bytes32{}, 0, 0, 0)

	proto := prototype.New(thor.BytesToAddress([]byte("proto")), st)
	binding := proto.Bind(thor.BytesToAddress([]byte("binding")))

	user := thor.BytesToAddress([]byte("user"))
	planCredit := big.NewInt(100000)
	planRecRate := big.NewInt(2222)
	sponsor := thor.BytesToAddress([]byte("sponsor"))

	tests := []struct {
		fn       func() interface{}
		expected interface{}
		msg      string
	}{

		{func() interface{} { return M(binding.IsUser(user)) }, M(false, nil), "should not be user"},
		{func() interface{} { return binding.AddUser(user, 1) }, nil, ""},
		{func() interface{} { return M(binding.IsUser(user)) }, M(true, nil), "should be user"},
		{func() interface{} { return binding.RemoveUser(user) }, nil, ""},
		{func() interface{} { return M(binding.IsUser(user)) }, M(false, nil), "removed user should not a user"},

		{func() interface{} { return M(binding.CreditPlan()) }, M(&big.Int{}, &big.Int{}, nil), "should be zero plan"},
		{func() interface{} { return binding.SetCreditPlan(planCredit, planRecRate) }, nil, ""},
		{func() interface{} { return M(binding.CreditPlan()) }, M(planCredit, planRecRate, nil), "should set plan"},

		{func() interface{} { return binding.AddUser(user, 1) }, nil, ""},
		{func() interface{} { return M(binding.UserCredit(user, 1)) }, M(planCredit, nil), "should have credit"},
		{func() interface{} { return M(binding.UserCredit(user, 2)) }, M(planCredit, nil), "should have full credit"},

		{func() interface{} { return binding.SetUserCredit(user, &big.Int{}, 1) }, nil, ""},
		{func() interface{} { return M(binding.UserCredit(user, 2)) }, M(planRecRate, nil), "should recover credit"},
		{func() interface{} { return M(binding.UserCredit(user, 100000)) }, M(planCredit, nil), "should recover to full credit"},

		{func() interface{} { return M(binding.IsSponsor(sponsor)) }, M(false, nil), "should not be sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, true) }, nil, ""},
		{func() interface{} { return M(binding.IsSponsor(sponsor)) }, M(true, nil), "should be sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, false) }, nil, ""},
		{func() interface{} { return M(binding.IsSponsor(sponsor)) }, M(false, nil), "should not be sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, true) }, nil, ""},
		{func() interface{} { binding.SelectSponsor(sponsor); return nil }, nil, ""},
		{func() interface{} { return M(binding.CurrentSponsor()) }, M(sponsor, nil), "should be current sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, false) }, nil, ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.fn(), tt.msg)
	}
}
