// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package prototype_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin/prototype"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func M(a ...interface{}) []interface{} {
	return a
}

func TestPrototype(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)

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

		{func() interface{} { return binding.IsUser(user) }, false, "should not be user"},
		{func() interface{} { binding.AddUser(user, 1); return nil }, nil, ""},
		{func() interface{} { return binding.IsUser(user) }, true, "should be user"},
		{func() interface{} { binding.RemoveUser(user); return nil }, nil, ""},
		{func() interface{} { return binding.IsUser(user) }, false, "removed user should not a user"},

		{func() interface{} { return M(binding.CreditPlan()) }, []interface{}{&big.Int{}, &big.Int{}}, "should be zero plan"},
		{func() interface{} { binding.SetCreditPlan(planCredit, planRecRate); return nil }, nil, ""},
		{func() interface{} { return M(binding.CreditPlan()) }, []interface{}{planCredit, planRecRate}, "should set plan"},

		{func() interface{} { binding.AddUser(user, 1); return nil }, nil, ""},
		{func() interface{} { return binding.UserCredit(user, 1) }, planCredit, "should have credit"},
		{func() interface{} { return binding.UserCredit(user, 2) }, planCredit, "should have full credit"},

		{func() interface{} { binding.SetUserCredit(user, &big.Int{}, 1); return nil }, nil, ""},
		{func() interface{} { return binding.UserCredit(user, 2) }, planRecRate, "should recover credit"},
		{func() interface{} { return binding.UserCredit(user, 100000) }, planCredit, "should recover to full credit"},

		{func() interface{} { return binding.IsSponsor(sponsor) }, false, "should not be sponsor"},
		{func() interface{} { binding.Sponsor(sponsor, true); return nil }, nil, ""},
		{func() interface{} { return binding.IsSponsor(sponsor) }, true, "should be sponsor"},
		{func() interface{} { binding.Sponsor(sponsor, false); return nil }, nil, ""},
		{func() interface{} { return binding.IsSponsor(sponsor) }, false, "should not be sponsor"},
		{func() interface{} { binding.Sponsor(sponsor, true); return nil }, nil, ""},
		{func() interface{} { binding.SelectSponsor(sponsor); return nil }, nil, ""},
		{func() interface{} { return binding.CurrentSponsor() }, sponsor, "should be current sponsor"},
		{func() interface{} { binding.Sponsor(sponsor, false); return nil }, nil, ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.fn(), tt.msg)
	}

	assert.Nil(t, st.Err())
}
