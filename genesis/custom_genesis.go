// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/thor"
)

// CustomGenesis is user customized genesis
type CustomGenesis struct {
	LaunchTime uint64           `json:"launchTime"`
	GasLimit   uint64           `json:"gaslimit"`
	ExtraData  string           `json:"extraData"`
	Accounts   []Account        `json:"accounts"`
	Authority  []Authority      `json:"authority"`
	Params     Params           `json:"params"`
	Executor   Executor         `json:"executor"`
	ForkConfig *thor.ForkConfig `json:"forkConfig"`
	Config     *thor.Config     `json:"config"`
}

// Account is the account will set to the genesis block
type Account struct {
	Address thor.Address            `json:"address"`
	Balance *HexOrDecimal256        `json:"balance"`
	Energy  *HexOrDecimal256        `json:"energy"`
	Code    string                  `json:"code"`
	Storage map[string]thor.Bytes32 `json:"storage"`
}

// Authority is the authority node info
type Authority struct {
	MasterAddress   thor.Address `json:"masterAddress"`
	EndorsorAddress thor.Address `json:"endorsorAddress"`
	Identity        thor.Bytes32 `json:"identity"`
}

// Executor is the params for executor info
type Executor struct {
	Approvers []Approver `json:"approvers"`
}

// Approver is the approver info for executor contract
type Approver struct {
	Address  thor.Address `json:"address"`
	Identity thor.Bytes32 `json:"identity"`
}

// Params means the chain params for params contract
type Params struct {
	RewardRatio         *HexOrDecimal256 `json:"rewardRatio"`
	BaseGasPrice        *HexOrDecimal256 `json:"baseGasPrice"`
	ProposerEndorsement *HexOrDecimal256 `json:"proposerEndorsement"`
	ExecutorAddress     *thor.Address    `json:"executorAddress"`
	MaxBlockProposers   *uint64          `json:"maxBlockProposers"`
}

// HexOrDecimal256 marshals big.Int as hex or decimal.
// Copied from go-ethereum/common/math and implement json. Marshaler
type HexOrDecimal256 math.HexOrDecimal256

// UnmarshalJSON implements the json.Unmarshaler interface.
func (i *HexOrDecimal256) UnmarshalJSON(input []byte) error {
	var hex string
	if err := json.Unmarshal(input, &hex); err != nil {
		if err = (*big.Int)(i).UnmarshalJSON(input); err != nil {
			return err
		}
		return nil
	}
	bigint, ok := math.ParseBig256(hex)
	if !ok {
		return fmt.Errorf("invalid hex or decimal integer %q", input)
	}
	*i = HexOrDecimal256(*bigint)
	return nil
}

// MarshalJSON implements the json.Marshaler interface.
func (i HexOrDecimal256) MarshalJSON() ([]byte, error) {
	decimal256 := math.HexOrDecimal256(i)
	text, err := decimal256.MarshalText()
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(text))
}
