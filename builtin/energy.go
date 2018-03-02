package builtin

import (
	"math/big"

	"github.com/vechain/thor/builtin/sslot"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// Energy binder of `Energy` contract.
var Energy = func() *energy {
	c := loadContract("Energy")
	return &energy{
		c,
		sslot.NewMap(c.Address, 100),
		sslot.NewArray(c.Address, 101),
		sslot.NewMap(c.Address, 102),
		sslot.NewMap(c.Address, 103),
		sslot.New(c.Address, 104),
		sslot.New(c.Address, 105),
		sslot.New(c.Address, 106),
		sslot.NewMap(c.Address, 107),
	}
}()

type energy struct {
	*contract

	accounts             *sslot.Map
	growthRates          *sslot.Array
	consumptionApprovals *sslot.Map
	masters              *sslot.Map
	tokenSupply          *sslot.StorageSlot
	totalSub             *sslot.StorageSlot
	totalAdd             *sslot.StorageSlot
	suppliers            *sslot.Map
}

var bigE18 = big.NewInt(1e18)

func (e *energy) SetTokenSupply(state *state.State, supply *big.Int) {
	e.tokenSupply.Save(state, supply)
}

// GetTotalSupply returns total supply of energy.
func (e *energy) GetTotalSupply(state *state.State, blockTime uint64) *big.Int {
	var tokenSupply big.Int
	e.tokenSupply.Load(state, &tokenSupply)

	// calc grown energy for total token supply
	grown := e.calcBalance(state, blockTime, energyAccount{
		&big.Int{},
		0,
		&tokenSupply,
	})

	var totalAdd, totalSub big.Int
	e.totalAdd.Load(state, &totalAdd)
	e.totalSub.Load(state, &totalSub)
	grown.Add(grown, &totalAdd)
	return grown.Sub(grown, &totalSub)
}

func (e *energy) GetTotalBurned(state *state.State) *big.Int {
	var totalAdd, totalSub big.Int
	e.totalAdd.Load(state, &totalAdd)
	e.totalSub.Load(state, &totalSub)
	return new(big.Int).Sub(&totalSub, &totalAdd)
}

func (e *energy) calcBalance(state *state.State, blockTime uint64, acc energyAccount) *big.Int {
	if acc.Timestamp >= blockTime {
		// never occur in real env.
		return acc.Balance
	}

	if acc.TokenBalance.Sign() == 0 {
		return acc.Balance
	}

	rateCount := e.growthRates.Len(state)

	t2 := blockTime
	newBalance := new(big.Int).Set(acc.Balance)

	// reversedly iterates rates
	for i := rateCount; i > 0; i-- {
		var rate energyGrowthRate
		e.growthRates.ForIndex(i-1).Load(state, &rate)

		t1 := rate.Timestamp
		if t1 < acc.Timestamp {
			t1 = acc.Timestamp
		}

		if t1 > t2 {
			// never occur in real env.
			return acc.Balance
		}

		if t1 != t2 && acc.TokenBalance.Sign() != 0 && rate.Rate.Sign() != 0 {
			// energy growth (token * rate * dt / 1e18)
			x := new(big.Int).SetUint64(t2 - t1)
			x.Mul(x, rate.Rate)
			x.Mul(x, acc.TokenBalance)
			x.Div(x, bigE18)
			newBalance.Add(newBalance, x)
		}

		t2 = rate.Timestamp

		if acc.Timestamp >= rate.Timestamp {
			break
		}
	}
	return newBalance
}

// GetBalance returns energy balance of an account at given block time.
func (e *energy) GetBalance(state *state.State, blockTime uint64, addr thor.Address) *big.Int {
	var acc energyAccount
	e.accounts.ForKey(addr).Load(state, &acc)
	return e.calcBalance(state, blockTime, acc)
}

func (e *energy) AddBalance(state *state.State, blockTime uint64, addr thor.Address, amount *big.Int) {
	bal := e.GetBalance(state, blockTime, addr)
	if amount.Sign() != 0 {
		bal.Add(bal, amount)

		var totalAdd big.Int
		e.totalAdd.Load(state, &totalAdd)
		totalAdd.Add(&totalAdd, amount)
		e.totalAdd.Save(state, &totalAdd)
	}
	e.accounts.ForKey(addr).Save(state, &energyAccount{
		Balance:      bal,
		Timestamp:    blockTime,
		TokenBalance: state.GetBalance(addr),
	})
}

func (e *energy) SubBalance(state *state.State, blockTime uint64, addr thor.Address, amount *big.Int) bool {
	bal := e.GetBalance(state, blockTime, addr)
	if bal.Cmp(amount) < 0 {
		return false
	}
	if amount.Sign() != 0 {
		bal.Sub(bal, amount)

		var totalSub big.Int
		e.totalSub.Load(state, &totalSub)
		totalSub.Add(&totalSub, amount)
		e.totalSub.Save(state, &totalSub)
	}
	e.accounts.ForKey(addr).Save(state, &energyAccount{
		Balance:      bal,
		Timestamp:    blockTime,
		TokenBalance: state.GetBalance(addr),
	})
	return true
}

func (e *energy) AdjustGrowthRate(state *state.State, blockTime uint64, rate *big.Int) {
	e.growthRates.Append(state, &energyGrowthRate{Rate: rate, Timestamp: blockTime})
}

func (e *energy) ApproveConsumption(state *state.State, blockTime uint64, contractAddr thor.Address, caller thor.Address,
	credit *big.Int, recoveryRate *big.Int, expiration uint64) {
	e.consumptionApprovals.ForKey([]interface{}{contractAddr, caller}).Save(state, &energyConsumptionApproval{
		Credit:       credit,
		RecoveryRate: recoveryRate,
		Expiration:   expiration,
		Timestamp:    blockTime,
		Remained:     credit,
	})
}

func (e *energy) GetConsumptionAllowance(state *state.State, blockTime uint64, contractAddr thor.Address, caller thor.Address) *big.Int {
	var ca energyConsumptionApproval
	e.consumptionApprovals.ForKey([]interface{}{contractAddr, caller}).Load(state, &ca)
	return ca.RemainedAt(blockTime)
}

func (e *energy) SetSupplier(state *state.State, contractAddr thor.Address, supplier thor.Address, agreed bool) {
	e.suppliers.ForKey(contractAddr).Save(state, energySupplier{
		supplier,
		agreed,
	})
}

func (e *energy) GetSupplier(state *state.State, contractAddr thor.Address) (thor.Address, bool) {
	var supplier energySupplier
	e.suppliers.ForKey(contractAddr).Load(state, &supplier)
	return supplier.Address, supplier.Agreed
}

func (e *energy) consumeContract(state *state.State, blockTime uint64, contractAddr thor.Address, caller thor.Address, amount *big.Int) (payer thor.Address, ok bool) {
	entry := e.consumptionApprovals.ForKey([]interface{}{contractAddr, caller})
	var ca energyConsumptionApproval
	entry.Load(state, &ca)
	remained := ca.RemainedAt(blockTime)
	if remained.Cmp(amount) < 0 {
		return thor.Address{}, false
	}

	defer func() {
		if ok {
			ca.Remained.Sub(remained, amount)
			ca.Timestamp = blockTime
			entry.Save(state, &ca)
		}
	}()

	var supplier energySupplier
	e.suppliers.ForKey(contractAddr).Load(state, &supplier)
	if supplier.Agreed {
		if e.SubBalance(state, blockTime, supplier.Address, amount) {
			return supplier.Address, true
		}
	}

	if e.SubBalance(state, blockTime, contractAddr, amount) {
		return contractAddr, true
	}
	return thor.Address{}, false
}

func (e *energy) Consume(state *state.State, blockTime uint64, contractAddr thor.Address, caller thor.Address, amount *big.Int) (thor.Address, bool) {
	if payer, ok := e.consumeContract(state, blockTime, contractAddr, caller, amount); ok {
		return payer, true
	}
	if e.SubBalance(state, blockTime, caller, amount) {
		return caller, true
	}
	return thor.Address{}, false
}

func (e *energy) SetContractMaster(state *state.State, contractAddr thor.Address, master thor.Address) {
	e.masters.ForKey(contractAddr).Save(state, master)
}

func (e *energy) GetContractMaster(state *state.State, contractAddr thor.Address) (master thor.Address) {
	e.masters.ForKey(contractAddr).Load(state, &master)
	return
}
