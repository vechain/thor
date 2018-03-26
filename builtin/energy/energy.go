package energy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	tokenSupplyKey = thor.Hash(crypto.Keccak256Hash([]byte("token-supply")))
	growthRatesKey = thor.Hash(crypto.Keccak256Hash([]byte("growth-rates")))
	totalAddKey    = thor.Hash(crypto.Keccak256Hash([]byte("total-add")))
	totalSubKey    = thor.Hash(crypto.Keccak256Hash([]byte("total-sub")))
)

func accountKey(addr thor.Address) thor.Hash {
	return thor.BytesToHash(append([]byte("a"), addr.Bytes()...))
}

func consumptionApprovalKey(contractAddr thor.Address, caller thor.Address) thor.Hash {
	return thor.Hash(crypto.Keccak256Hash(contractAddr.Bytes(), caller.Bytes()))
}

func supplierKey(contractAddr thor.Address) thor.Hash {
	return thor.BytesToHash(append([]byte("s"), contractAddr.Bytes()...))
}
func contractMasterKey(contractAddr thor.Address) thor.Hash {
	return thor.BytesToHash(append([]byte("m"), contractAddr.Bytes()...))
}

type Energy struct {
	addr  thor.Address
	state *state.State
}

func New(addr thor.Address, state *state.State) *Energy {
	return &Energy{addr, state}
}

func (e *Energy) getStorage(key thor.Hash, val interface{}) {
	e.state.GetStructedStorage(e.addr, key, val)
}

func (e *Energy) setStorage(key thor.Hash, val interface{}) {
	e.state.SetStructedStorage(e.addr, key, val)
}

func (e *Energy) SetTokenSupply(supply *big.Int) {
	e.setStorage(tokenSupplyKey, supply)
}

// GetTotalSupply returns total supply of energy.
func (e *Energy) GetTotalSupply(blockTime uint64) *big.Int {
	var tokenSupply big.Int
	e.getStorage(tokenSupplyKey, &tokenSupply)

	// calc grown energy for total token supply
	grown := (&account{&big.Int{}, 0, &tokenSupply}).CalcBalance(blockTime, e.getGrowthRates())

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

func (e *Energy) getAccount(addr thor.Address) *account {
	var acc account
	e.getStorage(accountKey(addr), &acc)
	return &acc
}

func (e *Energy) getOrSetAccount(addr thor.Address, cb func(*account) bool) bool {
	key := accountKey(addr)
	var acc account

	e.getStorage(key, &acc)
	if !cb(&acc) {
		return false
	}
	e.setStorage(key, &acc)
	return true
}

func (e *Energy) AdjustGrowthRate(blockTime uint64, rate *big.Int) {
	var rates growthRates
	e.getStorage(growthRatesKey, &rates)
	rates = append(rates, &growthRate{rate, blockTime})
	e.setStorage(growthRatesKey, rates)
}

func (e *Energy) getGrowthRates() growthRates {
	var rates growthRates
	e.getStorage(growthRatesKey, &rates)
	return rates
}

// GetBalance returns energy balance of an account at given block time.
func (e *Energy) GetBalance(blockTime uint64, addr thor.Address) *big.Int {
	return e.getAccount(addr).CalcBalance(blockTime, e.getGrowthRates())
}

func (e *Energy) AddBalance(blockTime uint64, addr thor.Address, amount *big.Int) {
	e.getOrSetAccount(addr, func(acc *account) bool {
		bal := acc.CalcBalance(blockTime, e.getGrowthRates())
		if amount.Sign() != 0 {
			bal.Add(bal, amount)

			var totalAdd big.Int
			e.getStorage(totalAddKey, &totalAdd)
			totalAdd.Add(&totalAdd, amount)
			e.setStorage(totalAddKey, &totalAdd)
		}
		*acc = account{
			Balance:      bal,
			Timestamp:    blockTime,
			TokenBalance: e.state.GetBalance(addr),
		}
		return true
	})
}

func (e *Energy) SubBalance(blockTime uint64, addr thor.Address, amount *big.Int) bool {
	return e.getOrSetAccount(addr, func(acc *account) bool {
		bal := acc.CalcBalance(blockTime, e.getGrowthRates())
		if bal.Cmp(amount) < 0 {
			return false
		}
		if amount.Sign() != 0 {
			bal.Sub(bal, amount)

			var totalSub big.Int
			e.getStorage(totalSubKey, &totalSub)
			totalSub.Add(&totalSub, amount)
			e.setStorage(totalSubKey, &totalSub)
		}
		*acc = account{
			Balance:      bal,
			Timestamp:    blockTime,
			TokenBalance: e.state.GetBalance(addr),
		}
		return true
	})
}

func (e *Energy) ApproveConsumption(
	blockTime uint64,
	contractAddr thor.Address,
	caller thor.Address,
	credit *big.Int,
	recoveryRate *big.Int,
	expiration uint64) {
	e.setStorage(consumptionApprovalKey(contractAddr, caller), &consumptionApproval{
		Credit:       credit,
		RecoveryRate: recoveryRate,
		Expiration:   expiration,
		Timestamp:    blockTime,
		Remained:     credit,
	})
}

func (e *Energy) GetConsumptionAllowance(
	blockTime uint64,
	contractAddr thor.Address,
	caller thor.Address) *big.Int {
	var ca consumptionApproval
	e.getStorage(consumptionApprovalKey(contractAddr, caller), &ca)
	return ca.RemainedAt(blockTime)
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
	blockTime uint64,
	contractAddr thor.Address,
	caller thor.Address,
	amount *big.Int) (payer thor.Address, ok bool) {

	caKey := consumptionApprovalKey(contractAddr, caller)
	var ca consumptionApproval
	e.getStorage(caKey, &ca)

	remained := ca.RemainedAt(blockTime)
	if remained.Cmp(amount) < 0 {
		return thor.Address{}, false
	}

	defer func() {
		if ok {
			ca.Remained.Sub(remained, amount)
			ca.Timestamp = blockTime
			e.setStorage(caKey, &ca)
		}
	}()

	var s supplier
	e.getStorage(supplierKey(contractAddr), &s)
	if s.Agreed {
		if e.SubBalance(blockTime, s.Address, amount) {
			return s.Address, true
		}
	}

	if e.SubBalance(blockTime, contractAddr, amount) {
		return contractAddr, true
	}
	return thor.Address{}, false
}

func (e *Energy) Consume(blockTime uint64, contractAddr thor.Address, caller thor.Address, amount *big.Int) (thor.Address, bool) {
	if payer, ok := e.consumeContract(blockTime, contractAddr, caller, amount); ok {
		return payer, true
	}
	if e.SubBalance(blockTime, caller, amount) {
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
