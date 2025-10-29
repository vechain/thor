// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/builtin/authority"
	"github.com/vechain/thor/v2/builtin/energy"
	"github.com/vechain/thor/v2/builtin/gascharger"
	"github.com/vechain/thor/v2/builtin/gen"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/prototype"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/xenv"
)

// Builtin contracts binding.
var (
	Params    = &paramsContract{mustLoadContract("Params")}
	Authority = &authorityContract{mustLoadContract("Authority")}
	Energy    = &energyContract{mustLoadContract("Energy")}
	Executor  = &executorContract{mustLoadContract("Executor")}
	Prototype = &prototypeContract{mustLoadContract("Prototype")}
	Extension = &extensionContract{
		mustLoadContract("Extension"),
		mustLoadContract("ExtensionV2"),
		mustLoadContract("ExtensionV3"),
	}
	Staker          = &stakerContract{mustLoadContract("Staker")}
	Measure         = mustLoadContract("Measure")
	Stargate        = &stargateContract{mustLoadContract("Stargate")}       // 0x0000000000000000000000005374617267617465
	StargateNFT     = &stargateNFTContract{mustLoadContract("StargateNFT")} // 0x00000000000000000053746172676174654e4654
	StargateProxy   = mustLoadContract("StargateProxy")
	ClockLib        = mustLoadContract("ClockLib")        // 0x000000000000000000000000436C6F636B4C6962
	LevelsLib       = mustLoadContract("LevelsLib")       // 0x00000000000000000000004c6576656c734C6962
	MintingLogicLib = mustLoadContract("MintingLogicLib") // 0x00000000004D696e74696E674c6f6769634C6962
	SettingsLib     = mustLoadContract("SettingsLib")     // 0x00000000000000000053657474696E67734C6962
	TokenLib        = mustLoadContract("TokenLib")        // 0x000000000000000000000000546F6b656e4C6962
	TokenManagerLib = mustLoadContract("TokenManagerLib") // 0x0000000000546f6b656e4D616E616765724C6962

	// return gas map maintains the builtin contracts that can be made native call cheaper
	// only the 0.4.24 compiled contracts are allowed to return gas, as the newer compiler
	// versions have dynamic cost and pattern that is not predictable, any further added new
	// builtin contract must not be added in this map.
	returnGas = map[thor.Address]bool{
		Params.Address:    true,
		Authority.Address: true,
		Energy.Address:    true,
		Prototype.Address: true,
		Extension.Address: true,
	}
)

type (
	paramsContract    struct{ *contract }
	authorityContract struct{ *contract }
	energyContract    struct{ *contract }
	executorContract  struct{ *contract }
	prototypeContract struct{ *contract }
	extensionContract struct {
		*contract
		V2 *contract
		V3 *contract
	}
	stakerContract      struct{ *contract }
	stargateContract    struct{ *contract }
	stargateNFTContract struct{ *contract }
)

func (p *paramsContract) Native(state *state.State) *params.Params {
	return params.New(p.Address, state)
}

func (a *authorityContract) Native(state *state.State) *authority.Authority {
	return authority.New(a.Address, state)
}

func (e *energyContract) Native(state *state.State, blockTime uint64) *energy.Energy {
	return energy.New(e.Address, state, blockTime, Params.Native(state))
}

func (p *prototypeContract) Native(state *state.State) *prototype.Prototype {
	return prototype.New(p.Address, state)
}

func (p *prototypeContract) Events() *abi.ABI {
	asset := "compiled/PrototypeEvent.abi"
	data := gen.MustABI(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(errors.Wrap(err, "load ABI for "+asset))
	}
	return abi
}

func (s *stakerContract) NativeMetered(state *state.State, charger *gascharger.Charger) *staker.Staker {
	return staker.New(s.Address, state, Params.Native(state), charger)
}

func (s *stakerContract) Native(state *state.State) *staker.Staker {
	return s.NativeMetered(state, nil)
}

func (s *stakerContract) Events() *abi.ABI {
	asset := "compiled/Staker.abi"
	data := gen.MustABI(asset)
	abi, err := abi.New(data)
	if err != nil {
		panic(errors.Wrap(err, "load ABI for "+asset))
	}
	return abi
}

type nativeMethod struct {
	abi *abi.Method
	run func(env *xenv.Environment) []any
}

type methodKey struct {
	thor.Address
	abi.MethodID
}

// Level represents a single NFT or staking level configuration.
type Level struct {
	Name                     string   // Name of the level (e.g., "Thunder", "Mjolnir")
	IsX                      bool     // Whether the level is for X-tokens
	ID                       uint8    // ID to identify the level, as a continuation of the legacy strength levels
	MaturityBlocks           uint64   // Maturity period in blocks
	ScaledRewardFactor       uint64   // Reward multiplier for that level scaled by 100 (i.e., 1.5 becomes 150)
	VetAmountRequiredToStake *big.Int // VET amount required for staking
}

// LevelAndSupply links a Level with its circulating supply and cap.
type LevelAndSupply struct {
	Level             Level  // Level details
	CirculatingSupply uint32 // Current circulating supply (Solidity uint208 → big.Int)
	Cap               uint32 // Maximum supply cap
}

// StargateInitializeV1Params represents the initialization parameters for Stargate V1.
type StargatNFTInitializeV1Params struct {
	TokenCollectionName   string           // ERC721 token collection name
	TokenCollectionSymbol string           // ERC721 token collection symbol
	BaseTokenURI          string           // Base URI for the token metadata
	Admin                 thor.Address     // Access control: Default admin address
	Upgrader              thor.Address     // Access control: Upgrader address
	Pauser                thor.Address     // Access control: Pauser address
	LevelOperator         thor.Address     // Access control: Level operator address
	LegacyNodes           thor.Address     // Address of the legacy TokenAuction contract
	StargateDelegation    thor.Address     // Address of the Stargate delegation contract
	VthoToken             thor.Address     // Address of the VTHO token contract
	LegacyLastTokenId     uint64           // Last token ID minted in the legacy TokenAuction contract
	LevelsAndSupplies     []LevelAndSupply // A list of levels and their supply
}

var nativeMethods = make(map[methodKey]*nativeMethod)

// FindNativeCall find native calls.
func FindNativeCall(to thor.Address, input []byte) (*abi.Method, func(*xenv.Environment) []any, bool, bool) {
	methodID, err := abi.ExtractMethodID(input)
	if err != nil {
		return nil, nil, false, false
	}

	method := nativeMethods[methodKey{to, methodID}]
	if method == nil {
		return nil, nil, false, false
	}
	return method.abi, method.run, true, returnGas[to]
}
