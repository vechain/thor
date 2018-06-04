// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package prototype

import (
	"math/big"

	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type Prototype struct {
	addr  thor.Address
	state *state.State
}

func New(addr thor.Address, state *state.State) *Prototype {
	return &Prototype{addr, state}
}

func (p *Prototype) Bind(self thor.Address) *Binding {
	return &Binding{p, self}
}

type Binding struct {
	prototype *Prototype
	self      thor.Address
}

func (b *Binding) userKey(user thor.Address) thor.Bytes32 {
	return thor.Blake2b(b.self.Bytes(), user.Bytes(), []byte("user"))
}
func (b *Binding) userPlanKey() thor.Bytes32 {
	return thor.Blake2b(b.self.Bytes(), []byte("user-plan"))
}

func (b *Binding) sponsorKey(sponsor thor.Address) thor.Bytes32 {
	return thor.Blake2b(b.self.Bytes(), sponsor.Bytes(), []byte("sponsor"))
}

func (b *Binding) curSponsorKey() thor.Bytes32 {
	return thor.Blake2b(b.self.Bytes(), []byte("cur-sponsor"))
}

func (b *Binding) getStorage(key thor.Bytes32, val interface{}) {
	b.prototype.state.GetStructuredStorage(b.prototype.addr, key, val)
}

func (b *Binding) setStorage(key thor.Bytes32, val interface{}) {
	b.prototype.state.SetStructuredStorage(b.prototype.addr, key, val)
}

func (b *Binding) Master() (master thor.Address) {
	master = b.prototype.state.GetMaster(b.self)
	return
}

func (b *Binding) SetMaster(master thor.Address) {
	b.prototype.state.SetMaster(b.self, master)
}

func (b *Binding) IsUser(user thor.Address) bool {
	var uo userObject
	b.getStorage(b.userKey(user), &uo)
	return !uo.IsEmpty()
}

func (b *Binding) AddUser(user thor.Address, blockTime uint64) {
	var up userPlan
	b.getStorage(b.userPlanKey(), &up)
	b.setStorage(b.userKey(user), &userObject{
		up.Credit,
		blockTime,
	})
}

func (b *Binding) RemoveUser(user thor.Address) {
	userKey := b.userKey(user)
	b.setStorage(userKey, uint8(0))
}

func (b *Binding) UserCredit(user thor.Address, blockTime uint64) *big.Int {
	var uo userObject
	b.getStorage(b.userKey(user), &uo)
	if uo.IsEmpty() {
		return &big.Int{}
	}
	var up userPlan
	b.getStorage(b.userPlanKey(), &up)
	return uo.Credit(&up, blockTime)
}

func (b *Binding) SetUserCredit(user thor.Address, credit *big.Int, blockTime uint64) {
	b.setStorage(b.userKey(user), &userObject{credit, blockTime})
}

func (b *Binding) UserPlan() (credit, recoveryRate *big.Int) {
	var up userPlan
	b.getStorage(b.userPlanKey(), &up)
	return up.Credit, up.RecoveryRate
}

func (b *Binding) SetUserPlan(credit, recoveryRate *big.Int) {
	b.setStorage(b.userPlanKey(), &userPlan{credit, recoveryRate})
}

func (b *Binding) Sponsor(sponsor thor.Address, yesOrNo bool) {
	sponsorKey := b.sponsorKey(sponsor)
	if yesOrNo {
		b.setStorage(sponsorKey, uint8(1))
	} else {
		b.setStorage(sponsorKey, uint8(0))
		if b.CurrentSponsor() == sponsor {
			b.setStorage(b.curSponsorKey(), thor.Address{})
		}
	}
}

func (b *Binding) IsSponsor(sponsor thor.Address) bool {
	var flag uint8
	b.getStorage(b.sponsorKey(sponsor), &flag)
	return flag != 0
}

func (b *Binding) SelectSponsor(sponsor thor.Address) {
	b.setStorage(b.curSponsorKey(), sponsor)
}

func (b *Binding) CurrentSponsor() (addr thor.Address) {
	b.getStorage(b.curSponsorKey(), &addr)
	return
}
