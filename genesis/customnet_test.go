// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis_test

import (
	"encoding/json"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
)

func CustomNetWithParams(
	t *testing.T,
	executor genesis.Executor,
	baseGasPrice genesis.HexOrDecimal256,
	rewardRatio genesis.HexOrDecimal256,
	proposerEndorsement genesis.HexOrDecimal256,
) genesis.CustomGenesis {
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
	config := thor.Config{
		HayabusaTP:                 math.MaxUint32,
		BlockInterval:              thor.BlockInterval(),
		EpochLength:                thor.EpochLength(),
		SeederInterval:             thor.SeederInterval(),
		ValidatorEvictionThreshold: thor.ValidatorEvictionThreshold(),
		LowStakingPeriod:           thor.LowStakingPeriod(),
		MediumStakingPeriod:        thor.MediumStakingPeriod(),
		HighStakingPeriod:          thor.HighStakingPeriod(),
		CooldownPeriod:             thor.CooldownPeriod(),
	}
	thor.SetConfig(config, true)

	devAccounts := genesis.DevAccounts()

	auth := make([]genesis.Authority, 0, len(devAccounts))
	for _, acc := range devAccounts {
		auth = append(auth, genesis.Authority{
			MasterAddress:   acc.Address,
			EndorsorAddress: acc.Address,
			Identity:        thor.BytesToBytes32([]byte("master")),
		})
	}

	accounts := make([]genesis.Account, 2)

	accounts[0].Balance = &genesis.HexOrDecimal256{}
	accounts[0].Energy = &genesis.HexOrDecimal256{}
	accounts[0].Code = "0x608060405234801561001057600080fd5b50606460008190555061017f806100286000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80632f5f3b3c14610046578063a32a3ee414610064578063acfee28314610082575b600080fd5b61004e61009e565b60405161005b91906100d0565b60405180910390f35b61006c6100a4565b60405161007991906100d0565b60405180910390f35b61009c6004803603810190610097919061011c565b6100ad565b005b60005481565b60008054905090565b8060008190555050565b6000819050919050565b6100ca816100b7565b82525050565b60006020820190506100e560008301846100c1565b92915050565b600080fd5b6100f9816100b7565b811461010457600080fd5b50565b600081359050610116816100f0565b92915050565b600060208284031215610132576101316100eb565b5b600061014084828501610107565b9150509291505056fea2646970667358221220a1012465f7be855f040e95566de3bbd50542ba31a7730d7fea2ef9de563a9ac164736f6c63430008110033"
	accounts[0].Storage = map[string]thor.Bytes32{
		"0x0000000000000000000000000000000000000000000000000000000000000001": thor.MustParseBytes32(
			"0x0000000000000000000000000000000000000000000000000000000000000002",
		),
	}

	mbp := uint64(10000)
	customGenesis := genesis.CustomGenesis{
		LaunchTime: 1526400000,
		GasLimit:   0,
		Executor:   executor,
		ExtraData:  "",
		ForkConfig: &defaultFC,
		Authority:  auth,
		Accounts:   accounts,
		Params: genesis.Params{
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
	customGenesis := CustomNetWithParams(t, genesis.Executor{}, genesis.HexOrDecimal256{}, genesis.HexOrDecimal256{}, genesis.HexOrDecimal256{})

	genesisBlock, err := genesis.NewCustomNet(&customGenesis)
	assert.NoError(t, err, "NewCustomNet should not return an error")
	assert.NotNil(t, genesisBlock, "NewCustomNet should return a non-nil Genesis object")
}

func TestNewCustomNetPanicInvalidApprovers(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()

	var approvers []genesis.Approver
	for range 1 {
		addr, _ := thor.ParseAddress("0x1f9090aaE28b8a3dCeaDf281B0F12828e676c326")

		approver := genesis.Approver{
			Address:  addr,
			Identity: thor.Bytes32{},
		}
		approvers = append(approvers, approver)
	}

	invalidExecutor := genesis.Executor{
		Approvers: approvers,
	}

	customGenesis := CustomNetWithParams(t, invalidExecutor, genesis.HexOrDecimal256{}, genesis.HexOrDecimal256{}, genesis.HexOrDecimal256{})

	// This call is expected to panic
	_, _ = genesis.NewCustomNet(&customGenesis)
}

func TestNewCustomNetInvalidBaseGas(t *testing.T) {
	baseGasPrice := genesis.HexOrDecimal256(*big.NewInt(-100))
	customGenesis := CustomNetWithParams(t, genesis.Executor{}, baseGasPrice, genesis.HexOrDecimal256{}, genesis.HexOrDecimal256{})

	genesisBlock, err := genesis.NewCustomNet(&customGenesis)
	assert.Error(t, err, "NewCustomNet should return an error")
	assert.Nil(t, genesisBlock, "NewCustomNet should return a nil Genesis object")
}

func TestNewCustomNetInvalidRewardRatio(t *testing.T) {
	rewardRatio := genesis.HexOrDecimal256(*big.NewInt(-100))
	customGenesis := CustomNetWithParams(t, genesis.Executor{}, genesis.HexOrDecimal256{}, rewardRatio, genesis.HexOrDecimal256{})

	genesisBlock, err := genesis.NewCustomNet(&customGenesis)
	assert.Error(t, err, "NewCustomNet should return an error")
	assert.Nil(t, genesisBlock, "NewCustomNet should return a nil Genesis object")
}

func TestNewCustomNetInvalidProposerEndorsement(t *testing.T) {
	proposerEndorsement := genesis.HexOrDecimal256(*big.NewInt(-100))
	customGenesis := CustomNetWithParams(t, genesis.Executor{}, genesis.HexOrDecimal256{}, genesis.HexOrDecimal256{}, proposerEndorsement)

	genesisBlock, err := genesis.NewCustomNet(&customGenesis)
	assert.Error(t, err, "NewCustomNet should return an error")
	assert.Nil(t, genesisBlock, "NewCustomNet should return a nil Genesis object")
}

func TestNewCustomGenesisMarshalUnmarshal(t *testing.T) {
	rewardRatio := genesis.HexOrDecimal256(*big.NewInt(-100))
	customGenesis := CustomNetWithParams(t, genesis.Executor{}, genesis.HexOrDecimal256{}, rewardRatio, genesis.HexOrDecimal256{})

	marshalVal, err := json.Marshal(customGenesis)
	assert.NoError(t, err, "Marshaling should not produce an error")

	expectedMarshal := `{"launchTime":1526400000,"gaslimit":0,"extraData":"","accounts":[{"address":"0x0000000000000000000000000000000000000000","balance":"0x0","energy":"0x0","code":"0x608060405234801561001057600080fd5b50606460008190555061017f806100286000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80632f5f3b3c14610046578063a32a3ee414610064578063acfee28314610082575b600080fd5b61004e61009e565b60405161005b91906100d0565b60405180910390f35b61006c6100a4565b60405161007991906100d0565b60405180910390f35b61009c6004803603810190610097919061011c565b6100ad565b005b60005481565b60008054905090565b8060008190555050565b6000819050919050565b6100ca816100b7565b82525050565b60006020820190506100e560008301846100c1565b92915050565b600080fd5b6100f9816100b7565b811461010457600080fd5b50565b600081359050610116816100f0565b92915050565b600060208284031215610132576101316100eb565b5b600061014084828501610107565b9150509291505056fea2646970667358221220a1012465f7be855f040e95566de3bbd50542ba31a7730d7fea2ef9de563a9ac164736f6c63430008110033","storage":{"0x0000000000000000000000000000000000000000000000000000000000000001":"0x0000000000000000000000000000000000000000000000000000000000000002"}},{"address":"0x0000000000000000000000000000000000000000","balance":null,"energy":null,"code":"","storage":null}],"authority":[{"masterAddress":"0xf077b491b355e64048ce21e3a6fc4751eeea77fa","endorsorAddress":"0xf077b491b355e64048ce21e3a6fc4751eeea77fa","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x435933c8064b4ae76be665428e0307ef2ccfbd68","endorsorAddress":"0x435933c8064b4ae76be665428e0307ef2ccfbd68","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x0f872421dc479f3c11edd89512731814d0598db5","endorsorAddress":"0x0f872421dc479f3c11edd89512731814d0598db5","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0xf370940abdbd2583bc80bfc19d19bc216c88ccf0","endorsorAddress":"0xf370940abdbd2583bc80bfc19d19bc216c88ccf0","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x99602e4bbc0503b8ff4432bb1857f916c3653b85","endorsorAddress":"0x99602e4bbc0503b8ff4432bb1857f916c3653b85","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x61e7d0c2b25706be3485980f39a3a994a8207acf","endorsorAddress":"0x61e7d0c2b25706be3485980f39a3a994a8207acf","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x361277d1b27504f36a3b33d3a52d1f8270331b8c","endorsorAddress":"0x361277d1b27504f36a3b33d3a52d1f8270331b8c","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0xd7f75a0a1287ab2916848909c8531a0ea9412800","endorsorAddress":"0xd7f75a0a1287ab2916848909c8531a0ea9412800","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0xabef6032b9176c186f6bf984f548bda53349f70a","endorsorAddress":"0xabef6032b9176c186f6bf984f548bda53349f70a","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"},{"masterAddress":"0x865306084235bf804c8bba8a8d56890940ca8f0b","endorsorAddress":"0x865306084235bf804c8bba8a8d56890940ca8f0b","identity":"0x00000000000000000000000000000000000000000000000000006d6173746572"}],"params":{"rewardRatio":"-0x64","baseGasPrice":"0x0","proposerEndorsement":"0x0","executorAddress":null,"maxBlockProposers":10000},"executor":{"approvers":null},"forkConfig":{"VIP191":4294967295,"ETH_CONST":4294967295,"BLOCKLIST":4294967295,"ETH_IST":4294967295,"VIP214":4294967295,"FINALITY":0,"HAYABUSA":4294967295,"GALACTICA":4294967295},"config":{"blockInterval":10,"epochLength":180,"seederInterval":8640,"validatorEvictionThreshold":60480,"lowStakingPeriod":60480,"mediumStakingPeriod":129600,"highStakingPeriod":259200,"cooldownPeriod":8640,"hayabusaTP":4294967295}}`
	assert.Equal(t, expectedMarshal, string(marshalVal))
}

func TestHexOrDecimal256MarshalUnmarshal(t *testing.T) {
	// Example hex string representing the value 100
	originalHex := `"0x64"` // Note the enclosing double quotes for valid JSON string

	// Unmarshal JSON into HexOrDecimal256
	var unmarshaledValue genesis.HexOrDecimal256

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

	var unmarshaledValue genesis.HexOrDecimal256
	err := unmarshaledValue.UnmarshalJSON([]byte(originalHex))
	assert.NoError(t, err, "Unmarshaling should not produce an error")
}
