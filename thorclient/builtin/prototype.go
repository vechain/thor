// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thor/v2/thorclient/httpclient"
)

type Prototype struct {
	contract bind.Contract
}

func NewPrototype(client *httpclient.Client) (*Prototype, error) {
	contract, err := bind.NewContract(client, builtin.Prototype.RawABI(), &builtin.Prototype.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to create prototype contract: %w", err)
	}
	return &Prototype{
		contract: contract,
	}, nil
}

func (p *Prototype) Raw() bind.Contract {
	return p.contract
}

// Master returns the master address for the given contract
func (p *Prototype) Master(contract thor.Address) (thor.Address, error) {
	out := new(common.Address)
	if err := p.contract.Method("master", contract).Call().Into(&out); err != nil {
		return thor.Address{}, err
	}
	return thor.Address(*out), nil
}

// SetMaster sets a new master for the contract
func (p *Prototype) SetMaster(self thor.Address, newMaster thor.Address) bind.MethodBuilder {
	return p.contract.Method("setMaster", self, newMaster)
}

// IsUser checks if the given address is a user of the contract
func (p *Prototype) IsUser(self thor.Address, user thor.Address) (bool, error) {
	out := new(bool)
	if err := p.contract.Method("isUser", self, user).Call().Into(&out); err != nil {
		return false, err
	}
	return *out, nil
}

// AddUser adds a user to the contract
func (p *Prototype) AddUser(self thor.Address, user thor.Address) bind.MethodBuilder {
	return p.contract.Method("addUser", self, user)
}

// RemoveUser removes a user from the contract
func (p *Prototype) RemoveUser(self thor.Address, user thor.Address) bind.MethodBuilder {
	return p.contract.Method("removeUser", self, user)
}

// UserCredit returns the credit amount for a specific user
func (p *Prototype) UserCredit(self thor.Address, user thor.Address) (*big.Int, error) {
	out := new(big.Int)
	if err := p.contract.Method("userCredit", self, user).Call().Into(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreditPlan returns the credit plan for the contract
func (p *Prototype) CreditPlan(self thor.Address) (*big.Int, *big.Int, error) {
	var out = [2]any{}
	out[0] = new(*big.Int)
	out[1] = new(*big.Int)
	if err := p.contract.Method("creditPlan", self).Call().Into(&out); err != nil {
		return nil, nil, err
	}
	return *(out[0].(**big.Int)), *(out[1].(**big.Int)), nil
}

// SetCreditPlan sets the credit plan for the contract
func (p *Prototype) SetCreditPlan(self thor.Address, credit *big.Int, recoveryRate *big.Int) bind.MethodBuilder {
	return p.contract.Method("setCreditPlan", self, credit, recoveryRate)
}

// CurrentSponsor returns the current sponsor address
func (p *Prototype) CurrentSponsor(self thor.Address) (thor.Address, error) {
	out := new(common.Address)
	if err := p.contract.Method("currentSponsor", self).Call().Into(&out); err != nil {
		return thor.Address{}, err
	}
	return thor.Address(*out), nil
}

// IsSponsor checks if the given address is a sponsor
func (p *Prototype) IsSponsor(self thor.Address, sponsor thor.Address) (bool, error) {
	out := new(bool)
	if err := p.contract.Method("isSponsor", self, sponsor).Call().Into(&out); err != nil {
		return false, err
	}
	return *out, nil
}

// SelectSponsor selects a sponsor for the contract
func (p *Prototype) SelectSponsor(self thor.Address, sponsor thor.Address) bind.MethodBuilder {
	return p.contract.Method("selectSponsor", self, sponsor)
}

// Sponsor sponsors the contract
func (p *Prototype) Sponsor(self thor.Address) bind.MethodBuilder {
	return p.contract.Method("sponsor", self)
}

// Unsponsor removes sponsorship from the contract
func (p *Prototype) Unsponsor(self thor.Address) bind.MethodBuilder {
	return p.contract.Method("unsponsor", self)
}

// Balance returns the balance at a specific block number
func (p *Prototype) Balance(self thor.Address, blockNumber *big.Int) (*big.Int, error) {
	out := new(big.Int)
	if err := p.contract.Method("balance", self, blockNumber).Call().Into(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Energy returns the energy at a specific block number
func (p *Prototype) Energy(self thor.Address, blockNumber *big.Int) (*big.Int, error) {
	out := new(big.Int)
	if err := p.contract.Method("energy", self, blockNumber).Call().Into(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// HasCode checks if the contract has code
func (p *Prototype) HasCode(self thor.Address) (bool, error) {
	out := new(bool)
	if err := p.contract.Method("hasCode", self).Call().Into(&out); err != nil {
		return false, err
	}
	return *out, nil
}

// StorageFor returns the storage value for a given key
func (p *Prototype) StorageFor(self thor.Address, key thor.Bytes32) (thor.Bytes32, error) {
	out := new(common.Hash)
	if err := p.contract.Method("storageFor", self, key).Call().Into(out); err != nil {
		return thor.Bytes32{}, err
	}
	return thor.Bytes32(*out), nil
}
