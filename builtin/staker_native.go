// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

func isContractPaused(state *state.State, pauseBit int) (bool, error) {
	switches, err := Params.Native(state).Get(thor.KeyStargateSwitches)
	if err != nil {
		return false, err
	}
	return switches.Bit(pauseBit) == 1, nil
}

func IsStargatePaused(state *state.State) error {
	isPaused, err := isContractPaused(state, 0)
	if err != nil {
		return err
	}
	if isPaused {
		return errors.New("stargate is paused")
	}
	return nil
}

// The staker pause switch at binary position 1. (binary: 1 [1] 0)
func IsStakerPaused(state *state.State) error {
	isPaused, err := isContractPaused(state, 1)
	if err != nil {
		return err
	}
	if isPaused {
		return errors.New("staker is paused")
	}
	return nil
}

func init() {
	defines := []struct {
		name string
		run  func(env *xenv.Environment) []any
	}{
		{"native_issuance", func(env *xenv.Environment) []any {
			staker := Staker.Native(env.State())
			issuance, err := Energy.Native(env.State(), env.BlockContext().Time).CalculateRewards(staker)
			if err != nil {
				return []any{new(big.Int), fmt.Sprintf("revert: %v", err)}
			}
			return []any{issuance, ""}
		}},
		{"native_delegatorContract", func(env *xenv.Environment) []any {
			params := Params.Native(env.State())
			contractBig, err := params.Get(thor.KeyStargateContractAddress)
			if err != nil {
				panic(fmt.Sprintf("failed to get contract address: %v", err))
			}
			delegatorContract := thor.BytesToAddress(contractBig.Bytes())
			return []any{delegatorContract}
		}},
		{"native_stakingPeriods", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)
			env.UseGas(thor.SloadGas)

			Staker.Native(env.State())
			return []any{
				staker.LowStakingPeriod.Get(),
				staker.MediumStakingPeriod.Get(),
				staker.HighStakingPeriod.Get(),
			}
		}},
		{"native_epochLength", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)

			Staker.Native(env.State())
			return []any{staker.EpochLength.Get(),}
		}},
		{"native_cooldownPeriod", func(env *xenv.Environment) []any {
			env.UseGas(thor.SloadGas)

			Staker.Native(env.State())
			return []any{staker.CooldownPeriod.Get()}
		}},
	}
	stakerAbi := Staker.NativeABI()
	for _, def := range defines {
		if method, found := stakerAbi.MethodByName(def.name); found {
			nativeMethods[methodKey{Staker.Address, method.ID()}] = &nativeMethod{
				abi: method,
				run: def.run,
			}
		} else {
			panic("method not found: " + def.name)
		}
	}
}
