// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package prototype

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
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
	return &Binding{p.addr, p.state, self}
}

type Binding struct {
	addr  thor.Address
	state *state.State
	self  thor.Address
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

func (b *Binding) getUserObject(user thor.Address) *userObject {
	var uo userObject
	b.state.DecodeStorage(b.addr, b.userKey(user), func(raw []byte) error {
		if len(raw) == 0 {
			uo = userObject{&big.Int{}, 0}
			return nil
		}
		return rlp.DecodeBytes(raw, &uo)
	})
	return &uo
}

func (b *Binding) setUserObject(user thor.Address, uo *userObject) {
	b.state.EncodeStorage(b.addr, b.userKey(user), func() ([]byte, error) {
		if uo.IsEmpty() {
			return nil, nil
		}
		return rlp.EncodeToBytes(uo)
	})
}

func (b *Binding) getUserPlan() *userPlan {
	var up userPlan
	b.state.DecodeStorage(b.addr, b.userPlanKey(), func(raw []byte) error {
		if len(raw) == 0 {
			up = userPlan{&big.Int{}, &big.Int{}}
			return nil
		}
		return rlp.DecodeBytes(raw, &up)
	})
	return &up
}

func (b *Binding) setUserPlan(up *userPlan) {
	b.state.EncodeStorage(b.addr, b.userPlanKey(), func() ([]byte, error) {
		if up.IsEmpty() {
			return nil, nil
		}
		return rlp.EncodeToBytes(up)
	})
}

func (b *Binding) IsUser(user thor.Address) bool {
	return !b.getUserObject(user).IsEmpty()
}

func (b *Binding) AddUser(user thor.Address, blockTime uint64) {
	b.setUserObject(user, &userObject{&big.Int{}, blockTime})
}

func (b *Binding) RemoveUser(user thor.Address) {
	// set to empty
	b.setUserObject(user, &userObject{&big.Int{}, 0})
}

func (b *Binding) UserCredit(user thor.Address, blockTime uint64) *big.Int {
	uo := b.getUserObject(user)
	if uo.IsEmpty() {
		return &big.Int{}
	}
	return uo.Credit(b.getUserPlan(), blockTime)
}

func (b *Binding) SetUserCredit(user thor.Address, credit *big.Int, blockTime uint64) {
	up := b.getUserPlan()
	used := new(big.Int).Sub(up.Credit, credit)
	if used.Sign() < 0 {
		used = &big.Int{}
	}
	b.setUserObject(user, &userObject{used, blockTime})
}

func (b *Binding) UserPlan() (credit, recoveryRate *big.Int) {
	up := b.getUserPlan()
	return up.Credit, up.RecoveryRate
}

func (b *Binding) SetUserPlan(credit, recoveryRate *big.Int) {
	b.setUserPlan(&userPlan{credit, recoveryRate})
}

func (b *Binding) Sponsor(sponsor thor.Address, yesOrNo bool) {
	b.state.EncodeStorage(b.addr, b.sponsorKey(sponsor), func() ([]byte, error) {
		if !yesOrNo {
			return nil, nil
		}
		return rlp.EncodeToBytes(&yesOrNo)
	})
}

func (b *Binding) IsSponsor(sponsor thor.Address) bool {
	var yesOrNo bool
	b.state.DecodeStorage(b.addr, b.sponsorKey(sponsor), func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}
		return rlp.DecodeBytes(raw, &yesOrNo)
	})
	return yesOrNo
}

func (b *Binding) SelectSponsor(sponsor thor.Address) {
	b.state.SetStorage(b.addr, b.curSponsorKey(), thor.BytesToBytes32(sponsor.Bytes()))
}

func (b *Binding) CurrentSponsor() thor.Address {
	return thor.BytesToAddress(b.state.GetStorage(b.addr, b.curSponsorKey()).Bytes())
}
