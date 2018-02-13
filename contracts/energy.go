package contracts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethparams "github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/contracts/rabi"
	"github.com/vechain/thor/contracts/sslot"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm/evm"
)

// Energy binder of `Energy` contract.
var Energy = func() *energy {
	addr := thor.BytesToAddress([]byte("eng"))
	abi := mustLoadABI("compiled/Energy.abi")
	return &energy{
		addr,
		abi,
		rabi.New(abi),
		sslot.NewMap(addr, 100),
		sslot.New(addr, 101),
		sslot.NewMap(addr, 102),
		sslot.NewMap(addr, 103),
	}
}()

type energy struct {
	Address     thor.Address
	ABI         *abi.ABI
	rabi        *rabi.ReversedABI
	accounts    *sslot.Map
	growthRates *sslot.StorageSlot
	sharings    *sslot.Map
	masters     *sslot.Map
}

var bigE18 = big.NewInt(1e18)

// RuntimeBytecodes load runtime byte codes.
func (e *energy) RuntimeBytecodes() []byte {
	return mustLoadHexData("compiled/Energy.bin-runtime")
}

func (e *energy) GetTotalSupply(state *state.State) *big.Int {
	return &big.Int{}
}

func (e *energy) GetBalance(state *state.State, blockTime uint64, addr thor.Address) *big.Int {
	var acc energyAccount
	e.accounts.ForKey(addr).Load(state, &acc)
	if acc.Timestamp >= blockTime {
		return acc.Balance
	}

	rates := e.GetGrowthRates(state)
	if len(rates) == 0 {
		return acc.Balance
	}

	t := blockTime
	x := &big.Int{}
	newBalance := new(big.Int).Set(acc.Balance)
	for i := len(rates) - 1; i >= 0; i++ {
		rate := rates[i]

		// energy growth (vet * rate * dt / 1e18)
		x.SetUint64(t - maxUInt64(acc.Timestamp, rate.Timestamp))
		x.Mul(x, rate.Rate)
		x.Mul(x, acc.VETBalance)
		x.Div(x, bigE18)
		newBalance.Add(newBalance, x)

		t = rate.Timestamp

		if acc.Timestamp >= rate.Timestamp {
			break
		}
	}
	return newBalance
}

func (e *energy) SetBalance(state *state.State, blockTime uint64, addr thor.Address, balance *big.Int) {
	e.accounts.ForKey(addr).Save(state, &energyAccount{
		Balance:    balance,
		Timestamp:  blockTime,
		VETBalance: state.GetBalance(addr),
	})
}

func (e *energy) GetGrowthRates(state *state.State) (rates energyGrowthRates) {
	e.growthRates.Load(state, &rates)
	return
}

func (e *energy) AdjustGrowthRate(state *state.State, blockTime uint64, rate *big.Int) {
	var rates energyGrowthRates
	e.growthRates.Load(state, &rates)
	rates = append(rates, energyGrowthRate{Rate: rate, Timestamp: blockTime})
	e.growthRates.Save(state, rates)
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

func (e *energy) Consume(state *state.State, blockTime uint64, caller thor.Address, callee thor.Address, amount *big.Int) (thor.Address, bool) {
	{
		calleeBalance := e.GetBalance(state, blockTime, callee)
		if calleeBalance.Cmp(amount) >= 0 {
			shareEntry := e.sharings.ForKey([]interface{}{callee, caller})
			var share energySharing
			shareEntry.Load(state, &share)
			remainedSharing := share.RemainedAt(blockTime)
			if remainedSharing.Cmp(amount) >= 0 {
				e.SetBalance(state, blockTime, callee, calleeBalance.Sub(calleeBalance, amount))

				share.Remained.Sub(remainedSharing, amount)
				share.Timestamp = blockTime
				shareEntry.Save(state, &share)
				return callee, true
			}
		}
	}
	{
		callerBalance := e.GetBalance(state, blockTime, caller)
		if callerBalance.Cmp(amount) >= 0 {
			e.SetBalance(state, blockTime, caller, callerBalance.Sub(callerBalance, amount))
			return caller, true
		}
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

// HandleNative helper method to hook VM contract calls.
func (e *energy) HandleNative(state *state.State, blockTime uint64, input []byte) func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
	name, err := e.rabi.NameOf(input)
	if err != nil {
		return nil
	}
	switch name {
	case "nativeGetExecutor":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas) {
				return nil, evm.ErrOutOfGas
			}
			return e.rabi.PackOutput(name, Executor.Address)
		}
	case "nativeGetTotalSupply":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas * 2) {
				return nil, evm.ErrOutOfGas
			}
			return e.rabi.PackOutput(name, e.GetTotalSupply(state))
		}
	case "nativeGetBalance":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas * 2) {
				return nil, evm.ErrOutOfGas
			}
			var addr common.Address
			if err := e.rabi.UnpackInput(&addr, name, input); err != nil {
				return nil, err
			}
			return e.rabi.PackOutput(name, e.GetBalance(state, blockTime, thor.Address(addr)))
		}
	case "nativeSetBalance":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if caller != e.Address {
				return nil, errNativeNotPermitted
			}
			if !useGas(ethparams.SstoreResetGas * 2) {
				return nil, evm.ErrOutOfGas
			}
			var args struct {
				Addr    common.Address
				Balance *big.Int
			}
			if err := e.rabi.UnpackInput(&args, name, input); err != nil {
				return nil, err
			}
			e.SetBalance(state, blockTime, thor.Address(args.Addr), args.Balance)
			return nil, nil
		}
	case "nativeAdjustGrowthRate":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if caller != e.Address {
				return nil, errNativeNotPermitted
			}
			if !useGas(ethparams.SstoreSetGas) {
				return nil, evm.ErrOutOfGas
			}
			var rate big.Int
			if err := e.rabi.UnpackInput(&rate, name, input); err != nil {
				return nil, err
			}
			e.AdjustGrowthRate(state, blockTime, &rate)
			return nil, nil
		}
	case "nativeSetSharing":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if caller != e.Address {
				return nil, errNativeNotPermitted
			}
			if !useGas(ethparams.SstoreSetGas) {
				return nil, evm.ErrOutOfGas
			}
			var args struct {
				From         common.Address
				To           common.Address
				Credit       *big.Int
				RecoveryRate *big.Int
				Expiration   uint64
			}
			if err := e.rabi.UnpackInput(&args, name, input); err != nil {
				return nil, err
			}
			e.SetSharing(state, blockTime,
				thor.Address(args.From), thor.Address(args.To), args.Credit, args.RecoveryRate, args.Expiration)
			return nil, nil
		}
	case "nativeGetSharingRemained":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas) {
				return nil, evm.ErrOutOfGas
			}
			var args struct {
				From common.Address
				To   common.Address
			}
			if err := e.rabi.UnpackInput(&args, name, input); err != nil {
				return nil, err
			}
			return e.rabi.PackOutput(name, e.GetSharingRemained(state, blockTime,
				thor.Address(args.From), thor.Address(args.To)))
		}
	case "nativeSetContractMaster":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if caller != e.Address {
				return nil, errNativeNotPermitted
			}
			if !useGas(ethparams.SstoreSetGas) {
				return nil, evm.ErrOutOfGas
			}
			var args struct {
				ContractAddr common.Address
				Master       common.Address
			}
			if err := e.rabi.UnpackInput(&args, name, input); err != nil {
				return nil, err
			}
			e.SetContractMaster(state, thor.Address(args.ContractAddr), thor.Address(args.Master))
			return nil, nil
		}
	case "nativeGetContractMaster":
		return func(useGas func(gas uint64) bool, caller thor.Address) ([]byte, error) {
			if !useGas(ethparams.SloadGas) {
				return nil, evm.ErrOutOfGas
			}
			var contractAddr common.Address
			if err := e.rabi.UnpackInput(&contractAddr, name, input); err != nil {
				return nil, err
			}
			return e.rabi.PackOutput(name, e.GetContractMaster(state, thor.Address(contractAddr)))
		}
	}
	return nil
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
	if isZeroBig(ea.Balance) &&
		ea.Timestamp == 0 &&
		isZeroBig(ea.VETBalance) {
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

type energyGrowthRates []energyGrowthRate

var _ state.StorageEncoder = (energyGrowthRates)(nil)
var _ state.StorageDecoder = (*energyGrowthRates)(nil)

func (egrs energyGrowthRates) Encode() ([]byte, error) {
	if len(egrs) == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(egrs)
}

func (egrs *energyGrowthRates) Decode(data []byte) error {
	if len(data) == 0 {
		*egrs = nil
		return nil
	}
	return rlp.DecodeBytes(data, egrs)
}

type energySharing struct {
	Credit       *big.Int
	RecoveryRate *big.Int
	Expiration   uint64
	Remained     *big.Int
	Timestamp    uint64
}

func (es *energySharing) Encode() ([]byte, error) {
	if isZeroBig(es.Credit) {
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

func isZeroBig(b *big.Int) bool {
	return b == nil || b.Sign() == 0
}

func maxUInt64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
