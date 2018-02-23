package builtin

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
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
	}
}()

type energy struct {
	*contract

	accounts    *sslot.Map
	growthRates *sslot.Array
	sharings    *sslot.Map
	masters     *sslot.Map
}

var bigE18 = big.NewInt(1e18)

// GetTotalSupply returns total supply of energy.
func (e *energy) GetTotalSupply(state *state.State) *big.Int {
	return &big.Int{}
}

// GetBalance returns energy balance of an account at given block time.
func (e *energy) GetBalance(state *state.State, blockTime uint64, addr thor.Address) *big.Int {
	var acc energyAccount
	e.accounts.ForKey(addr).Load(state, &acc)
	if acc.Timestamp >= blockTime {
		// never occur in real env.
		return acc.Balance
	}

	rateCount := e.growthRates.Len(state)

	t2 := blockTime
	newBalance := new(big.Int).Set(acc.Balance)
	for i := rateCount; i > 0; i-- {
		var rate energyGrowthRate
		e.growthRates.ForIndex(i-1).Load(state, &rate)

		t1 := maxUInt64(acc.Timestamp, rate.Timestamp)
		if t1 > t2 {
			// never occur in real env.
			return acc.Balance
		}

		if t1 != t2 && acc.VETBalance.Sign() != 0 && rate.Rate.Sign() != 0 {
			// energy growth (vet * rate * dt / 1e18)
			x := new(big.Int).SetUint64(t2 - t1)
			x.Mul(x, rate.Rate)
			x.Mul(x, acc.VETBalance)
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

// SetBalance set balance for an account.
func (e *energy) SetBalance(state *state.State, blockTime uint64, addr thor.Address, balance *big.Int) {
	e.accounts.ForKey(addr).Save(state, &energyAccount{
		Balance:    balance,
		Timestamp:  blockTime,
		VETBalance: state.GetBalance(addr),
	})
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

func (e *energy) consumeCaller(state *state.State, blockTime uint64, caller thor.Address, callee thor.Address, amount *big.Int) bool {
	// consume caller
	callerBalance := e.GetBalance(state, blockTime, caller)
	if callerBalance.Cmp(amount) < 0 {
		return false
	}
	e.SetBalance(state, blockTime, caller, callerBalance.Sub(callerBalance, amount))
	return true
}

func (e *energy) consumeCallee(state *state.State, blockTime uint64, caller thor.Address, callee thor.Address, amount *big.Int) bool {
	// try to consume callee's sharing
	calleeBalance := e.GetBalance(state, blockTime, callee)
	if calleeBalance.Cmp(amount) < 0 {
		return false
	}
	shareEntry := e.sharings.ForKey([]interface{}{callee, caller})
	var share energySharing
	shareEntry.Load(state, &share)
	remainedSharing := share.RemainedAt(blockTime)
	if remainedSharing.Cmp(amount) < 0 {
		return false
	}
	e.SetBalance(state, blockTime, callee, calleeBalance.Sub(calleeBalance, amount))

	share.Remained.Sub(remainedSharing, amount)
	share.Timestamp = blockTime
	shareEntry.Save(state, &share)
	return true
}

func (e *energy) Consume(state *state.State, blockTime uint64, caller thor.Address, callee thor.Address, amount *big.Int) (thor.Address, bool) {
	if e.consumeCallee(state, blockTime, caller, callee, amount) {
		return callee, true
	}
	if e.consumeCaller(state, blockTime, caller, callee, amount) {
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

type energyAccount struct {
	Balance *big.Int

	// snapshot
	Timestamp  uint64
	VETBalance *big.Int
}

var _ state.StorageEncoder = (*energyAccount)(nil)
var _ state.StorageDecoder = (*energyAccount)(nil)

func (ea *energyAccount) Encode() ([]byte, error) {
	if isBigZero(ea.Balance) &&
		ea.Timestamp == 0 &&
		isBigZero(ea.VETBalance) {
		return nil, nil
	}
	return rlp.EncodeToBytes(ea)
}

func (ea *energyAccount) Decode(data []byte) error {
	if len(data) == 0 {
		*ea = energyAccount{&big.Int{}, 0, &big.Int{}}
		return nil
	}
	return rlp.DecodeBytes(data, ea)
}

type energyGrowthRate struct {
	Rate      *big.Int
	Timestamp uint64
}

var _ state.StorageEncoder = (*energyGrowthRate)(nil)
var _ state.StorageDecoder = (*energyGrowthRate)(nil)

func (egr *energyGrowthRate) Encode() ([]byte, error) {
	if isBigZero(egr.Rate) && egr.Timestamp == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(egr)
}

func (egr *energyGrowthRate) Decode(data []byte) error {
	if len(data) == 0 {
		*egr = energyGrowthRate{&big.Int{}, 0}
		return nil
	}
	return rlp.DecodeBytes(data, egr)
}

type energySharing struct {
	Credit       *big.Int
	RecoveryRate *big.Int
	Expiration   uint64
	Remained     *big.Int
	Timestamp    uint64
}

func (es *energySharing) Encode() ([]byte, error) {
	if isBigZero(es.Credit) {
		return nil, nil
	}
	return rlp.EncodeToBytes(es)
}

func (es *energySharing) Decode(data []byte) error {
	if len(data) == 0 {
		*es = energySharing{&big.Int{}, &big.Int{}, 0, &big.Int{}, 0}
		return nil
	}
	return rlp.DecodeBytes(data, es)
}
func (es *energySharing) RemainedAt(blockTime uint64) *big.Int {
	if blockTime >= es.Expiration {
		return &big.Int{}
	}

	x := new(big.Int).SetUint64(blockTime - es.Timestamp)
	x.Mul(x, es.RecoveryRate)
	x.Add(x, es.Remained)
	if x.Cmp(es.Credit) < 0 {
		return x
	}
	return es.Credit
}

func isBigZero(b *big.Int) bool {
	return b == nil || b.Sign() == 0
}

func maxUInt64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
