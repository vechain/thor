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

	master := thor.BytesToAddress([]byte("master"))
	user := thor.BytesToAddress([]byte("user"))
	planCredit := big.NewInt(100000)
	planRecRate := big.NewInt(2222)
	sponsor := thor.BytesToAddress([]byte("sponsor"))

	tests := []struct {
		fn       func() interface{}
		expected interface{}
		msg      string
	}{
		{func() interface{} { return binding.Master() }, thor.Address{}, "should be empty master"},
		{func() interface{} { binding.SetMaster(master); return nil }, nil, ""},
		{func() interface{} { return binding.Master() }, master, "should set master"},

		{func() interface{} { return binding.IsUser(user) }, false, "should not be user"},
		{func() interface{} { return binding.AddUser(user, 1) }, true, "should add user"},
		{func() interface{} { return binding.AddUser(user, 1) }, false, "should fail to add user"},
		{func() interface{} { return binding.IsUser(user) }, true, "should be user"},
		{func() interface{} { return binding.RemoveUser(thor.BytesToAddress([]byte("not a user"))) }, false, "should not remove non-user"},
		{func() interface{} { return binding.RemoveUser(user) }, true, "should remove user"},
		{func() interface{} { return binding.IsUser(user) }, false, "removed user should not a user"},

		{func() interface{} { return M(binding.UserPlan()) }, []interface{}{&big.Int{}, &big.Int{}}, "should be zero plan"},
		{func() interface{} { binding.SetUserPlan(planCredit, planRecRate); return nil }, nil, ""},
		{func() interface{} { return M(binding.UserPlan()) }, []interface{}{planCredit, planRecRate}, "should set plan"},

		{func() interface{} { return binding.AddUser(user, 1) }, true, "should add user"},
		{func() interface{} { return binding.UserCredit(user, 1) }, planCredit, "should have credit"},
		{func() interface{} { return binding.UserCredit(user, 2) }, planCredit, "should have full credit"},

		{func() interface{} { binding.SetUserCredit(user, &big.Int{}, 1); return nil }, nil, ""},
		{func() interface{} { return binding.UserCredit(user, 2) }, planRecRate, "should recover credit"},
		{func() interface{} { return binding.UserCredit(user, 100000) }, planCredit, "should recover to full credit"},

		{func() interface{} { return binding.IsSponsor(sponsor) }, false, "should not be sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, true) }, true, "should set sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, true) }, false, "should not set sponsor"},
		{func() interface{} { return binding.IsSponsor(sponsor) }, true, "should be sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, false) }, true, "should unset sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, false) }, false, "should not unset sponsor"},
		{func() interface{} { return binding.IsSponsor(sponsor) }, false, "should not be sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, true) }, true, "should be sponsor"},
		{func() interface{} { return binding.SelectSponsor(sponsor) }, true, "should select sponsor"},
		{func() interface{} { return binding.CurrentSponsor() }, sponsor, "should be current sponsor"},
		{func() interface{} { return binding.Sponsor(sponsor, false) }, true, "should unset sponsor"},
		{func() interface{} { return binding.CurrentSponsor() }, thor.Address{}, "should be empty current sponsor"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.fn())
	}

	assert.Nil(t, st.Err())
}
