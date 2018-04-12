package energy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	tokenSupplyKey = thor.Bytes32(crypto.Keccak256Hash([]byte("token-supply")))
	totalAddKey    = thor.Bytes32(crypto.Keccak256Hash([]byte("total-add")))
	totalSubKey    = thor.Bytes32(crypto.Keccak256Hash([]byte("total-sub")))
)

func consumptionApprovalKey(contractAddr thor.Address, caller thor.Address) thor.Bytes32 {
	return thor.Bytes32(crypto.Keccak256Hash(contractAddr.Bytes(), caller.Bytes()))
}

func supplierKey(contractAddr thor.Address) thor.Bytes32 {
	return thor.BytesToBytes32(append([]byte("s"), contractAddr.Bytes()...))
}
func contractMasterKey(contractAddr thor.Address) thor.Bytes32 {
	return thor.BytesToBytes32(append([]byte("m"), contractAddr.Bytes()...))
}

type Energy struct {
	addr  thor.Address
	state *state.State
}

func New(addr thor.Address, state *state.State) *Energy {
	return &Energy{addr, state}
}

func (e *Energy) getStorage(key thor.Bytes32, val interface{}) {
	e.state.GetStructedStorage(e.addr, key, val)
}

func (e *Energy) setStorage(key thor.Bytes32, val interface{}) {
	e.state.SetStructedStorage(e.addr, key, val)
}

// InitializeTokenSupply initialize VET token supply info.
func (e *Energy) InitializeTokenSupply(supply *big.Int) {
	e.setStorage(tokenSupplyKey, supply)
}

// GetTotalSupply returns total supply of energy.
func (e *Energy) GetTotalSupply(blockNum uint32) *big.Int {
	var tokenSupply big.Int
	e.getStorage(tokenSupplyKey, &tokenSupply)
	var tokenSupplyTime uint64
	e.getStorage(tokenSupplyKey, &tokenSupplyTime)

	// calc grown energy for total token supply
	grown := (&account{Balance: &big.Int{}}).CalcBalance(&tokenSupply, blockNum)

	var totalAdd, totalSub big.Int
	e.getStorage(totalAddKey, &totalAdd)
	e.getStorage(totalSubKey, &totalSub)
	grown.Add(grown, &totalAdd)
	return grown.Sub(grown, &totalSub)
}

func (e *Energy) GetTotalBurned() *big.Int {
	var totalAdd, totalSub big.Int
	e.getStorage(totalAddKey, &totalAdd)
	e.getStorage(totalSubKey, &totalSub)
	return new(big.Int).Sub(&totalSub, &totalAdd)
}

// GetBalance returns energy balance of an account at given block time.
func (e *Energy) GetBalance(addr thor.Address, blockNum uint32) *big.Int {
	return e.state.GetEnergy(addr, blockNum)
}

func (e *Energy) AddBalance(addr thor.Address, blockNum uint32, amount *big.Int) {
	bal := e.state.GetEnergy(addr, blockNum)
	if amount.Sign() != 0 {
		var totalAdd big.Int
		e.getStorage(totalAddKey, &totalAdd)
		e.setStorage(totalAddKey, totalAdd.Add(&totalAdd, amount))

		e.state.SetEnergy(addr, blockNum, new(big.Int).Add(bal, amount))
	} else {
		e.state.SetEnergy(addr, blockNum, bal)
	}
}

func (e *Energy) SubBalance(addr thor.Address, blockNum uint32, amount *big.Int) bool {
	bal := e.state.GetEnergy(addr, blockNum)
	if amount.Sign() != 0 {
		if bal.Cmp(amount) < 0 {
			return false
		}
		var totalSub big.Int
		e.getStorage(totalSubKey, &totalSub)
		e.setStorage(totalSubKey, totalSub.Add(&totalSub, amount))

		e.state.SetEnergy(addr, blockNum, new(big.Int).Sub(bal, amount))
	} else {
		e.state.SetEnergy(addr, blockNum, bal)
	}
	return true
}

func (e *Energy) ApproveConsumption(
	blockNum uint32,
	contractAddr thor.Address,
	caller thor.Address,
	credit *big.Int,
	recoveryRate *big.Int,
	expiration uint32) {
	e.setStorage(consumptionApprovalKey(contractAddr, caller), &consumptionApproval{
		Credit:       credit,
		RecoveryRate: recoveryRate,
		Expiration:   expiration,
		BlockNum:     blockNum,
		Remained:     credit,
	})
}

func (e *Energy) GetConsumptionAllowance(
	blockNum uint32,
	contractAddr thor.Address,
	caller thor.Address) *big.Int {
	var ca consumptionApproval
	e.getStorage(consumptionApprovalKey(contractAddr, caller), &ca)
	return ca.RemainedAt(blockNum)
}

func (e *Energy) SetSupplier(contractAddr thor.Address, supplierAddr thor.Address, agreed bool) {
	e.setStorage(supplierKey(contractAddr), &supplier{
		supplierAddr,
		agreed,
	})
}

func (e *Energy) GetSupplier(contractAddr thor.Address) (thor.Address, bool) {
	var s supplier
	e.getStorage(supplierKey(contractAddr), &s)
	return s.Address, s.Agreed
}

func (e *Energy) consumeContract(
	blockNum uint32,
	contractAddr thor.Address,
	caller thor.Address,
	amount *big.Int) (payer thor.Address, ok bool) {

	caKey := consumptionApprovalKey(contractAddr, caller)
	var ca consumptionApproval
	e.getStorage(caKey, &ca)

	remained := ca.RemainedAt(blockNum)
	if remained.Cmp(amount) < 0 {
		return thor.Address{}, false
	}

	defer func() {
		if ok {
			ca.Remained.Sub(remained, amount)
			ca.BlockNum = blockNum
			e.setStorage(caKey, &ca)
		}
	}()

	var s supplier
	e.getStorage(supplierKey(contractAddr), &s)
	if s.Agreed {
		if e.SubBalance(s.Address, blockNum, amount) {
			return s.Address, true
		}
	}

	if e.SubBalance(contractAddr, blockNum, amount) {
		return contractAddr, true
	}
	return thor.Address{}, false
}

func (e *Energy) Consume(blockNum uint32, contractAddr *thor.Address, caller thor.Address, amount *big.Int) (thor.Address, bool) {
	if contractAddr != nil {
		if payer, ok := e.consumeContract(blockNum, *contractAddr, caller, amount); ok {
			return payer, true
		}
	}
	if e.SubBalance(caller, blockNum, amount) {
		return caller, true
	}
	return thor.Address{}, false
}

func (e *Energy) SetContractMaster(contractAddr thor.Address, master thor.Address) {
	e.setStorage(contractMasterKey(contractAddr), master)
}

func (e *Energy) GetContractMaster(contractAddr thor.Address) (master thor.Address) {
	e.getStorage(contractMasterKey(contractAddr), &master)
	return
}
