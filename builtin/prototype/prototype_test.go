// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package prototype

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func M(a ...any) []any {
	return a
}

func TestPrototype(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	proto := New(thor.BytesToAddress([]byte("proto")), st)
	binding := proto.Bind(thor.BytesToAddress([]byte("binding")))

	user := thor.BytesToAddress([]byte("user"))
	planCredit := big.NewInt(100000)
	planRecRate := big.NewInt(2222)
	sponsor := thor.BytesToAddress([]byte("sponsor"))

	tests := []struct {
		fn       func() any
		expected any
		msg      string
	}{
		{func() any { return M(binding.IsUser(user)) }, M(false, nil), "should not be user"},
		{func() any { return binding.AddUser(user, 1) }, nil, ""},
		{func() any { return M(binding.IsUser(user)) }, M(true, nil), "should be user"},
		{func() any { return binding.RemoveUser(user) }, nil, ""},
		{func() any { return M(binding.IsUser(user)) }, M(false, nil), "removed user should not a user"},

		{func() any { return M(binding.CreditPlan()) }, M(&big.Int{}, &big.Int{}, nil), "should be zero plan"},
		{func() any { return binding.SetCreditPlan(planCredit, planRecRate) }, nil, ""},
		{func() any { return M(binding.CreditPlan()) }, M(planCredit, planRecRate, nil), "should set plan"},

		{func() any { return binding.AddUser(user, 1) }, nil, ""},
		{func() any { return M(binding.UserCredit(user, 1)) }, M(planCredit, nil), "should have credit"},
		{func() any { return M(binding.UserCredit(user, 2)) }, M(planCredit, nil), "should have full credit"},

		{func() any { return binding.SetUserCredit(user, &big.Int{}, 1) }, nil, ""},
		{func() any { return M(binding.UserCredit(user, 2)) }, M(planRecRate, nil), "should recover credit"},
		{func() any { return M(binding.UserCredit(user, 100000)) }, M(planCredit, nil), "should recover to full credit"},

		{func() any { return M(binding.IsSponsor(sponsor)) }, M(false, nil), "should not be sponsor"},
		{func() any { return binding.Sponsor(sponsor, true) }, nil, ""},
		{func() any { return M(binding.IsSponsor(sponsor)) }, M(true, nil), "should be sponsor"},
		{func() any { return binding.Sponsor(sponsor, false) }, nil, ""},
		{func() any { return M(binding.IsSponsor(sponsor)) }, M(false, nil), "should not be sponsor"},
		{func() any { return binding.Sponsor(sponsor, true) }, nil, ""},
		{func() any { binding.SelectSponsor(sponsor); return nil }, nil, ""},
		{func() any { return M(binding.CurrentSponsor()) }, M(sponsor, nil), "should be current sponsor"},
		{func() any { return binding.Sponsor(sponsor, false) }, nil, ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.fn(), tt.msg)
	}
}
