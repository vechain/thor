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
	}
}()

type energy struct {
	*contract

	accounts    *sslot.Map
	growthRates *sslot.Array
	sharings    *sslot.Map
	masters     *sslot.Map
	tokenSupply *sslot.StorageSlot
	totalSub    *sslot.StorageSlot
	totalAdd    *sslot.StorageSlot
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

func (e *energy) SetSharing(state *state.State, blockTime uint64, from thor.Address, to thor.Address,
	credit *big.Int, recoveryRate *big.Int, expiration uint64) {
	e.sharings.ForKey([]interface{}{from, to}).Save(state, &energySharing{
		Credit:       credit,
		RecoveryRate: recoveryRate,
		Expiration:   expiration,
		Timestamp:    blockTime,
		Remained:     credit,
	})
}

func (e *energy) GetSharingRemained(state *state.State, blockTime uint64, from thor.Address, to thor.Address) *big.Int {
	var es energySharing
	e.sharings.ForKey([]interface{}{from, to}).Load(state, &es)
	return es.RemainedAt(blockTime)
}

func (e *energy) consumeCallee(state *state.State, blockTime uint64, caller thor.Address, callee thor.Address, amount *big.Int) bool {
	// try to consume callee's sharing
	shareEntry := e.sharings.ForKey([]interface{}{callee, caller})
	var share energySharing
	shareEntry.Load(state, &share)
	remainedSharing := share.RemainedAt(blockTime)
	if remainedSharing.Cmp(amount) < 0 {
		return false
	}

	if !e.SubBalance(state, blockTime, callee, amount) {
		return false
	}

	share.Remained.Sub(remainedSharing, amount)
	share.Timestamp = blockTime
	shareEntry.Save(state, &share)
	return true
}

func (e *energy) Consume(state *state.State, blockTime uint64, caller thor.Address, callee thor.Address, amount *big.Int) (thor.Address, bool) {
	if e.consumeCallee(state, blockTime, caller, callee, amount) {
		return callee, true
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
