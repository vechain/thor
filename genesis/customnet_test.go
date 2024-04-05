// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
)

func CustomNetWithParams(t *testing.T, executor genesis.Executor, baseGasPrice genesis.HexOrDecimal256, rewardRatio genesis.HexOrDecimal256, proposerEndorsement genesis.HexOrDecimal256) genesis.CustomGenesis {
	var defaultFC = thor.ForkConfig{
		VIP191:    math.MaxUint32,
		ETH_CONST: math.MaxUint32,
		BLOCKLIST: math.MaxUint32,
		ETH_IST:   math.MaxUint32,
		VIP214:    math.MaxUint32,
		FINALITY:  0,
	}

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
	for i := 0; i < 1; i++ {
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

func TestHexOrDecimal256MarshalUnmarshal(t *testing.T) {
	// Example hex string representing the value 100
	originalHex := `"0x64"` // Note the enclosing double quotes for valid JSON string

	// Unmarshal JSON into HexOrDecimal256
	var unmarshaledValue genesis.HexOrDecimal256
	err := unmarshaledValue.UnmarshalJSON([]byte(originalHex))
	assert.NoError(t, err, "Unmarshaling should not produce an error")

	// Marshal the value back to JSON
	marshaledJSON, err := unmarshaledValue.MarshalJSON()
	assert.NoError(t, err, "Marshaling should not produce an error")

	// Compare the original hex string with the marshaled JSON string
	assert.Equal(t, "0x64", string(marshaledJSON), "Original hex and marshaled JSON should be equivalent")
}

func TestHexOrDecimal256MarshalUnmarshalWithNilError(t *testing.T) {
	// Example hex string representing the value 100
	originalHex := "0x64"

	var unmarshaledValue genesis.HexOrDecimal256
	err := unmarshaledValue.UnmarshalJSON([]byte(originalHex))
	assert.NoError(t, err, "Unmarshaling should not produce an error")
}
