// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func newCustomNet(gen *CustomGenesis) (*Genesis, error) {
	if gen.Config != nil {
		if gen.Config.BlockInterval <= 1 {
			return nil, errors.New("BlockInterval can not be zero or one")
		}

		if gen.Config.EpochLength <= 1 {
			return nil, errors.New("EpochLength can not be zero or one")
		}

		thor.SetConfig(*gen.Config)
	}

	launchTime := gen.LaunchTime
	if gen.GasLimit == 0 {
		gen.GasLimit = thor.InitialGasLimit
	}
	var executor thor.Address
	if gen.Params.ExecutorAddress != nil {
		executor = *gen.Params.ExecutorAddress
	} else {
		executor = builtin.Executor.Address
	}

	builder := new(Builder).
		Timestamp(launchTime).
		GasLimit(gen.GasLimit).
		ForkConfig(gen.ForkConfig).
		State(func(state *state.State) error {
			// alloc builtin contracts
			if err := state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes()); err != nil {
				return err
			}

			// if executor is the default executor, set the executor code
			if executor == builtin.Executor.Address && len(gen.Executor.Approvers) > 0 {
				if err := state.SetCode(builtin.Executor.Address, builtin.Executor.RuntimeBytecodes()); err != nil {
					return err
				}
			}

			tokenSupply := &big.Int{}
			energySupply := &big.Int{}
			for _, a := range gen.Accounts {
				if b := (*big.Int)(a.Balance); b != nil {
					if b.Sign() < 0 {
						return fmt.Errorf("%s: balance must be a non-negative integer", a.Address)
					}
					tokenSupply.Add(tokenSupply, b)
					if err := state.SetBalance(a.Address, b); err != nil {
						return err
					}
					if err := state.SetEnergy(a.Address, &big.Int{}, launchTime); err != nil {
						return err
					}
				}
				if e := (*big.Int)(a.Energy); e != nil {
					if e.Sign() < 0 {
						return fmt.Errorf("%s: energy must be a non-negative integer", a.Address)
					}
					energySupply.Add(energySupply, e)
					if err := state.SetEnergy(a.Address, e, launchTime); err != nil {
						return err
					}
				}
				if len(a.Code) > 0 {
					code, err := hexutil.Decode(a.Code)
					if err != nil {
						return fmt.Errorf("invalid contract code for address: %s", a.Address)
					}
					if err := state.SetCode(a.Address, code); err != nil {
						return err
					}
				}
				if len(a.Storage) > 0 {
					for k, v := range a.Storage {
						state.SetStorage(a.Address, thor.MustParseBytes32(k), v)
					}
				}
			}

			return builtin.Energy.Native(state, launchTime).SetInitialSupply(tokenSupply, energySupply)
		})

	///// initialize builtin contracts

	// initialize params
	bgp := (*big.Int)(gen.Params.BaseGasPrice)
	if bgp != nil {
		if bgp.Sign() < 0 {
			return nil, errors.New("baseGasPrice must be a non-negative integer")
		}
	} else {
		bgp = thor.InitialBaseGasPrice
	}

	r := (*big.Int)(gen.Params.RewardRatio)
	if r != nil {
		if r.Sign() < 0 {
			return nil, errors.New("rewardRatio must be a non-negative integer")
		}
	} else {
		r = thor.InitialRewardRatio
	}

	e := (*big.Int)(gen.Params.ProposerEndorsement)
	if e != nil {
		if e.Sign() < 0 {
			return nil, errors.New("proposerEndorsement must a non-negative integer")
		}
	} else {
		e = thor.InitialProposerEndorsement
	}

	data := mustEncodeInput(builtin.Params.ABI, "set", thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), thor.Address{})

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyRewardRatio, r)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyLegacyTxBaseGasPrice, bgp)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyProposerEndorsement, e)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	if m := gen.Params.MaxBlockProposers; m != nil {
		if *m == uint64(0) {
			return nil, errors.New("maxBlockProposers must a non-negative integer")
		}
		data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyMaxBlockProposers, new(big.Int).SetUint64(*m))
		builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)
	}

	if len(gen.Authority) == 0 {
		return nil, errors.New("at least one authority node")
	}
	// add initial authority nodes
	for _, anode := range gen.Authority {
		data := mustEncodeInput(builtin.Authority.ABI, "add", anode.MasterAddress, anode.EndorsorAddress, anode.Identity)
		builder.Call(tx.NewClause(&builtin.Authority.Address).WithData(data), executor)
	}

	// if executor is the default executor, set the approvers
	if executor == builtin.Executor.Address && len(gen.Executor.Approvers) > 0 {
		for _, approver := range gen.Executor.Approvers {
			data := mustEncodeInput(builtin.Executor.ABI, "addApprover", approver.Address, approver.Identity)
			builder.Call(tx.NewClause(&builtin.Executor.Address).WithData(data), executor)
		}
	}

	if len(gen.ExtraData) > 0 {
		var extra [28]byte
		copy(extra[:], gen.ExtraData)
		builder.ExtraData(extra)
	}

	id, err := builder.ComputeID()
	if err != nil {
		panic(err)
	}
	return &Genesis{builder, id, "customnet"}, nil
}

func CustomNetWithParams(
	t *testing.T,
	executor Executor,
	baseGasPrice HexOrDecimal256,
	rewardRatio HexOrDecimal256,
	proposerEndorsement HexOrDecimal256,
) CustomGenesis {
	defaultFC := thor.ForkConfig{
		VIP191:    math.MaxUint32,
		ETH_CONST: math.MaxUint32,
		BLOCKLIST: math.MaxUint32,
		ETH_IST:   math.MaxUint32,
		VIP214:    math.MaxUint32,
		FINALITY:  0,
		GALACTICA: math.MaxUint32,
		HAYABUSA:  math.MaxUint32,
	}
	hayabusaTP := uint32(math.MaxUint32)
	config := thor.Config{
		HayabusaTP:                 &hayabusaTP,
		BlockInterval:              thor.BlockInterval(),
		EpochLength:                thor.EpochLength(),
		SeederInterval:             thor.SeederInterval(),
		ValidatorEvictionThreshold: thor.ValidatorEvictionThreshold(),
		LowStakingPeriod:           thor.LowStakingPeriod(),
		MediumStakingPeriod:        thor.MediumStakingPeriod(),
		HighStakingPeriod:          thor.HighStakingPeriod(),
		CooldownPeriod:             thor.CooldownPeriod(),
	}
	thor.SetConfig(config)

	devAccounts := DevAccounts()

	auth := make([]Authority, 0, len(devAccounts))
	for _, acc := range devAccounts {
		auth = append(auth, Authority{
			MasterAddress:   acc.Address,
			EndorsorAddress: acc.Address,
			Identity:        thor.BytesToBytes32([]byte("master")),
		})
	}

	accounts := make([]Account, 2)

	accounts[0].Balance = &HexOrDecimal256{}
	accounts[0].Energy = &HexOrDecimal256{}
	accounts[0].Code = "0x608060405234801561001057600080fd5b50606460008190555061017f806100286000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80632f5f3b3c14610046578063a32a3ee414610064578063acfee28314610082575b600080fd5b61004e61009e565b60405161005b91906100d0565b60405180910390f35b61006c6100a4565b60405161007991906100d0565b60405180910390f35b61009c6004803603810190610097919061011c565b6100ad565b005b60005481565b60008054905090565b8060008190555050565b6000819050919050565b6100ca816100b7565b82525050565b60006020820190506100e560008301846100c1565b92915050565b600080fd5b6100f9816100b7565b811461010457600080fd5b50565b600081359050610116816100f0565b92915050565b600060208284031215610132576101316100eb565b5b600061014084828501610107565b9150509291505056fea2646970667358221220a1012465f7be855f040e95566de3bbd50542ba31a7730d7fea2ef9de563a9ac164736f6c63430008110033"
	accounts[0].Storage = map[string]thor.Bytes32{
		"0x0000000000000000000000000000000000000000000000000000000000000001": thor.MustParseBytes32(
			"0x0000000000000000000000000000000000000000000000000000000000000002",
		),
	}

	mbp := uint64(10000)
	customGenesis := CustomGenesis{
		LaunchTime: 1526400000,
		GasLimit:   0,
		Executor:   executor,
		ExtraData:  "",
		ForkConfig: &defaultFC,
		Authority:  auth,
		Accounts:   accounts,
		Params: Params{
			MaxBlockProposers:   &mbp,
			BaseGasPrice:        &baseGasPrice,
			RewardRatio:         &rewardRatio,
			ProposerEndorsement: &proposerEndorsement,
		},
		Config: &config,
	}

	return customGenesis
}

func TestNewCustomNet(t *testing.T) {
	customGenesis := CustomNetWithParams(t, Executor{}, HexOrDecimal256{}, HexOrDecimal256{}, HexOrDecimal256{})

	genesisBlock, err := newCustomNet(&customGenesis)
	assert.NoError(t, err, "NewCustomNet should not return an error")
	assert.NotNil(t, genesisBlock, "NewCustomNet should return a non-nil Genesis object")
}

func TestNewCustomNetPanicInvalidApprovers(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()

	var approvers []Approver
	for range 1 {
		addr, _ := thor.ParseAddress("0x1f9090aaE28b8a3dCeaDf281B0F12828e676c326")

		approver := Approver{
			Address:  addr,
			Identity: thor.Bytes32{},
		}
		approvers = append(approvers, approver)
	}

	invalidExecutor := Executor{
		Approvers: approvers,
	}

	customGenesis := CustomNetWithParams(t, invalidExecutor, HexOrDecimal256{}, HexOrDecimal256{}, HexOrDecimal256{})

	// This call is expected to panic
	_, _ = newCustomNet(&customGenesis)
}

func TestNewCustomNetInvalidBaseGas(t *testing.T) {
	baseGasPrice := HexOrDecimal256(*big.NewInt(-100))
	customGenesis := CustomNetWithParams(t, Executor{}, baseGasPrice, HexOrDecimal256{}, HexOrDecimal256{})

	genesisBlock, err := newCustomNet(&customGenesis)
	assert.Error(t, err, "NewCustomNet should return an error")
	assert.Nil(t, genesisBlock, "NewCustomNet should return a nil Genesis object")
}

func TestNewCustomNetInvalidRewardRatio(t *testing.T) {
	rewardRatio := HexOrDecimal256(*big.NewInt(-100))
	customGenesis := CustomNetWithParams(t, Executor{}, HexOrDecimal256{}, rewardRatio, HexOrDecimal256{})

	genesisBlock, err := newCustomNet(&customGenesis)
	assert.Error(t, err, "NewCustomNet should return an error")
	assert.Nil(t, genesisBlock, "NewCustomNet should return a nil Genesis object")
}

func TestNewCustomNetInvalidProposerEndorsement(t *testing.T) {
	proposerEndorsement := HexOrDecimal256(*big.NewInt(-100))
	customGenesis := CustomNetWithParams(t, Executor{}, HexOrDecimal256{}, HexOrDecimal256{}, proposerEndorsement)

	genesisBlock, err := newCustomNet(&customGenesis)
	assert.Error(t, err, "NewCustomNet should return an error")
	assert.Nil(t, genesisBlock, "NewCustomNet should return a nil Genesis object")
}

func TestNewCustomGenesisMarshalUnmarshal(t *testing.T) {
	rewardRatio := HexOrDecimal256(*big.NewInt(-100))
	customGenesis := CustomNetWithParams(t, Executor{}, HexOrDecimal256{}, rewardRatio, HexOrDecimal256{})

	marshalVal, err := json.Marshal(customGenesis)
	assert.NoError(t, err, "Marshaling should not produce an error")

	expectedMarshal := `{"launchTime":1526400000,"gaslimit":0,"extraData":"","accounts":[{"address":"0x0000000000000000000000000000000000000000","balance":"0x0","energy":"0x0","code":"0x608060405234801561001057600080fd5b50606460008190555061017f806100286000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80632f5f3b3c14610046578063a32a3ee414610064578063acfee28314610082575b600080fd5b61004e61009e565b60405161005b91906100d0565b60405180910390f35b61006c6100a4565b60405161007991906100d0565b60405180910390f35b61009c6004803603810190610097919061011c565b6100ad565b005b60005481565b60008054905090565b8060008190555050565b6000819050919050565b6100ca816100b7565b82525050565b60006020820190506100e560008301846100c1565b92915050565b600080fd5b6100f9816100b7565b811461010457600080fd5b50565b600081359050610116816100f0565b92915050565b600060208284031215610132576101316100eb565b5b600061014084828501610107565b9150509291505056fea2646970667358221220a1012465f7be855f040e95566de3bbd50542ba31a7730d7fea2ef9de563a9ac164736f6c63430008110033","storage":{"0x0000000000000000000000000000000000000000000000000000000000000001":"0x0000000000000000000000000000000000000000000000000000000000000002"}},{"address":"0x0000000000000000000000000000000000000000","balance":null,"energy":null,"code":"","storage":null}],"authority":[{"masterAddress":"0xf077b491b355e64048ce21e3a6fc4751eeea77fa","endorsorAddress":"0xf077b491b355e64048ce21e3a6fc4751eeea77fa","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x435933c8064b4ae76be665428e0307ef2ccfbd68","endorsorAddress":"0x435933c8064b4ae76be665428e0307ef2ccfbd68","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x0f872421dc479f3c11edd89512731814d0598db5","endorsorAddress":"0x0f872421dc479f3c11edd89512731814d0598db5","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0xf370940abdbd2583bc80bfc19d19bc216c88ccf0","endorsorAddress":"0xf370940abdbd2583bc80bfc19d19bc216c88ccf0","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x99602e4bbc0503b8ff4432bb1857f916c3653b85","endorsorAddress":"0x99602e4bbc0503b8ff4432bb1857f916c3653b85","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x61e7d0c2b25706be3485980f39a3a994a8207acf","endorsorAddress":"0x61e7d0c2b25706be3485980f39a3a994a8207acf","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x361277d1b27504f36a3b33d3a52d1f8270331b8c","endorsorAddress":"0x361277d1b27504f36a3b33d3a52d1f8270331b8c","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0xd7f75a0a1287ab2916848909c8531a0ea9412800","endorsorAddress":"0xd7f75a0a1287ab2916848909c8531a0ea9412800","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0xabef6032b9176c186f6bf984f548bda53349f70a","endorsorAddress":"0xabef6032b9176c186f6bf984f548bda53349f70a","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x865306084235bf804c8bba8a8d56890940ca8f0b","endorsorAddress":"0x865306084235bf804c8bba8a8d56890940ca8f0b","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"}],"params":{"rewardRatio":"-0x64","baseGasPrice":"0x0","proposerEndorsement":"0x0","executorAddress":null,"maxBlockProposers":10000},"executor":{"approvers":null},"forkConfig":{"VIP191":4294967295,"ETH_CONST":4294967295,"BLOCKLIST":4294967295,"ETH_IST":4294967295,"VIP214":4294967295,"FINALITY":0,"HAYABUSA":4294967295,"GALACTICA":4294967295},"config":{"blockInterval":10,"epochLength":180,"seederInterval":8640,"validatorEvictionThreshold":60480,"lowStakingPeriod":60480,"mediumStakingPeriod":129600,"highStakingPeriod":259200,"cooldownPeriod":8640,"hayabusaTP":4294967295}}`
	assert.Equal(t, expectedMarshal, string(marshalVal))
}

func TestHexOrDecimal256MarshalUnmarshal(t *testing.T) {
	// Example hex string representing the value 100
	originalHex := `"0x64"` // Note the enclosing double quotes for valid JSON string

	// Unmarshal JSON into HexOrDecimal256
	var unmarshaledValue HexOrDecimal256

	// using direct function
	err := unmarshaledValue.UnmarshalJSON([]byte(originalHex))
	assert.NoError(t, err, "Unmarshaling should not produce an error")

	// using json overloading ( satisfies the json.Unmarshal interface )
	err = json.Unmarshal([]byte(originalHex), &unmarshaledValue)
	assert.NoError(t, err, "Unmarshaling should not produce an error")

	// Marshal the value back to JSON
	// using direct function
	directMarshallJSON, err := unmarshaledValue.MarshalJSON()
	assert.NoError(t, err, "Marshaling should not produce an error")
	assert.Equal(t, originalHex, string(directMarshallJSON))

	// using json overloading ( satisfies the json.Unmarshal interface )
	// using value
	marshalVal, err := json.Marshal(unmarshaledValue)
	assert.NoError(t, err, "Marshaling should not produce an error")
	assert.Equal(t, originalHex, string(marshalVal))

	// using json overloading ( satisfies the json.Unmarshal interface )
	// using pointer
	marshalPtr, err := json.Marshal(&unmarshaledValue)
	assert.NoError(t, err, "Marshaling should not produce an error")
	assert.Equal(t, originalHex, string(marshalPtr))
}

func TestHexOrDecimal256MarshalUnmarshalWithNilError(t *testing.T) {
	// Example hex string representing the value 100
	originalHex := "0x64"

	var unmarshaledValue HexOrDecimal256
	err := unmarshaledValue.UnmarshalJSON([]byte(originalHex))
	assert.NoError(t, err, "Unmarshaling should not produce an error")
}
