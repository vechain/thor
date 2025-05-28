package builtin

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
)

type Prototype struct {
	contract *bind.Caller
}

func NewPrototype(client *thorclient.Client) *Prototype {
	contract, err := bind.NewCaller(client, builtin.Prototype.RawABI(), builtin.Prototype.Address)
	if err != nil {
		panic(fmt.Sprintf("failed to create prototype contract: %v", err))
	}
	return &Prototype{
		contract: contract,
	}
}

func (p *Prototype) Raw() *bind.Caller {
	return p.contract
}

func (p *Prototype) Revision(id string) *Prototype {
	return &Prototype{
		contract: p.contract.Revision(id),
	}
}

// Master returns the master address for the given contract
func (p *Prototype) Master(self thor.Address) (thor.Address, error) {
	out := new(common.Address)
	if err := p.contract.CallInto("master", &out, self); err != nil {
		return thor.Address{}, err
	}
	return thor.Address(*out), nil
}

// SetMaster sets a new master for the contract
func (p *Prototype) SetMaster(signer bind.Signer, self thor.Address, newMaster thor.Address) *bind.Sender {
	return p.contract.Attach(signer).Sender("setMaster", self, newMaster)
}

// IsUser checks if the given address is a user of the contract
func (p *Prototype) IsUser(self thor.Address, user thor.Address) (bool, error) {
	out := new(bool)
	if err := p.contract.CallInto("isUser", &out, self, user); err != nil {
		return false, err
	}
	return *out, nil
}

// AddUser adds a user to the contract
func (p *Prototype) AddUser(signer bind.Signer, self thor.Address, user thor.Address) *bind.Sender {
	return p.contract.Attach(signer).Sender("addUser", self, user)
}

// RemoveUser removes a user from the contract
func (p *Prototype) RemoveUser(signer bind.Signer, self thor.Address, user thor.Address) *bind.Sender {
	return p.contract.Attach(signer).Sender("removeUser", self, user)
}

// UserCredit returns the credit amount for a specific user
func (p *Prototype) UserCredit(self thor.Address, user thor.Address) (*big.Int, error) {
	out := new(big.Int)
	if err := p.contract.CallInto("userCredit", &out, self, user); err != nil {
		return nil, err
	}
	return out, nil
}

// CreditPlan returns the credit plan for the contract
func (p *Prototype) CreditPlan(self thor.Address) (*big.Int, *big.Int, error) {
	var out = [2]any{}
	out[0] = new(*big.Int)
	out[1] = new(*big.Int)
	if err := p.contract.CallInto("creditPlan", &out, self); err != nil {
		return nil, nil, err
	}
	return *(out[0].(**big.Int)), *(out[1].(**big.Int)), nil
}

// SetCreditPlan sets the credit plan for the contract
func (p *Prototype) SetCreditPlan(signer bind.Signer, self thor.Address, credit *big.Int, recoveryRate *big.Int) *bind.Sender {
	return p.contract.Attach(signer).Sender("setCreditPlan", self, credit, recoveryRate)
}

// CurrentSponsor returns the current sponsor address
func (p *Prototype) CurrentSponsor(self thor.Address) (thor.Address, error) {
	out := new(common.Address)
	if err := p.contract.CallInto("currentSponsor", &out, self); err != nil {
		return thor.Address{}, err
	}
	return thor.Address(*out), nil
}

// IsSponsor checks if the given address is a sponsor
func (p *Prototype) IsSponsor(self thor.Address, sponsor thor.Address) (bool, error) {
	out := new(bool)
	if err := p.contract.CallInto("isSponsor", &out, self, sponsor); err != nil {
		return false, err
	}
	return *out, nil
}

// SelectSponsor selects a sponsor for the contract
func (p *Prototype) SelectSponsor(signer bind.Signer, self thor.Address, sponsor thor.Address) *bind.Sender {
	return p.contract.Attach(signer).Sender("selectSponsor", self, sponsor)
}

// Sponsor sponsors the contract
func (p *Prototype) Sponsor(signer bind.Signer, self thor.Address) *bind.Sender {
	return p.contract.Attach(signer).Sender("sponsor", self)
}

// Unsponsor removes sponsorship from the contract
func (p *Prototype) Unsponsor(signer bind.Signer, self thor.Address) *bind.Sender {
	return p.contract.Attach(signer).Sender("unsponsor", self)
}

// Balance returns the balance at a specific block number
func (p *Prototype) Balance(self thor.Address, blockNumber *big.Int) (*big.Int, error) {
	out := new(big.Int)
	if err := p.contract.CallInto("balance", &out, self, blockNumber); err != nil {
		return nil, err
	}
	return out, nil
}

// Energy returns the energy at a specific block number
func (p *Prototype) Energy(self thor.Address, blockNumber *big.Int) (*big.Int, error) {
	out := new(big.Int)
	if err := p.contract.CallInto("energy", &out, self, blockNumber); err != nil {
		return nil, err
	}
	return out, nil
}

// HasCode checks if the contract has code
func (p *Prototype) HasCode(self thor.Address) (bool, error) {
	out := new(bool)
	if err := p.contract.CallInto("hasCode", &out, self); err != nil {
		return false, err
	}
	return *out, nil
}

// StorageFor returns the storage value for a given key
func (p *Prototype) StorageFor(self thor.Address, key thor.Bytes32) (thor.Bytes32, error) {
	out := new(common.Hash)
	if err := p.contract.CallInto("storageFor", &out, self, key); err != nil {
		return thor.Bytes32{}, err
	}
	return thor.Bytes32(*out), nil
}
