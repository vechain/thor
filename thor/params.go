// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/params"
)

/*
 NOTE: any changes to gas limit or block interval may affect how the txIndex and blockNumber are stored in logdb/sequence.go:
  - an increase in gas limit may require more bits for txIndex;
  - if block frequency is increased, blockNumber will increment faster, potentially exhausting the allocated bits sooner than expected.
*/
// Constants of block chain.
const (
	TxGas                     uint64 = 5000
	ClauseGas                 uint64 = params.TxGas - TxGas
	ClauseGasContractCreation uint64 = params.TxGasContractCreation - TxGas

	MinGasLimit          uint64 = 1000 * 1000
	InitialGasLimit      uint64 = 10 * 1000 * 1000 // InitialGasLimit gas limit value int genesis block.
	GasLimitBoundDivisor uint64 = 1024             // from ethereum
	GetBalanceGas        uint64 = 400              // EIP158 gas table
	SloadGas             uint64 = 200              // EIP158 gas table
	SstoreSetGas         uint64 = params.SstoreSetGas
	SstoreResetGas       uint64 = params.SstoreResetGas

	MaxTxWorkDelay            uint32 = 30                     // (unit: block) if tx delay exceeds this value, no energy can be exchanged.
	TolerableBlockPackingTime        = 500 * time.Millisecond // the indicator to adjust target block gas limit
	MaxStateHistory                  = 65535                  // max guaranteed state history allowed to be accessed in EVM, presented in block number

	GasTargetPercentage      = 75                 // percentage of the block gas limit to determine the gas target
	InitialBaseFee           = 10_000_000_000_000 // 10^13 wei, 0.00001 VTHO
	BaseFeeChangeDenominator = 8                  // determines the percentage change in the base fee per block based on network utilization

	MaxPosScore = 10000 // max total score after PoS fork
)

// Keys of governance params.
var (
	KeyExecutorAddress           = BytesToBytes32([]byte("executor"))
	KeyRewardRatio               = BytesToBytes32([]byte("reward-ratio"))
	KeyValidatorRewardPercentage = BytesToBytes32([]byte("validator-reward-percentage"))
	KeyLegacyTxBaseGasPrice      = BytesToBytes32([]byte("base-gas-price")) // the legacy tx default gas price
	KeyProposerEndorsement       = BytesToBytes32([]byte("proposer-endorsement"))
	KeyMaxBlockProposers         = BytesToBytes32([]byte("max-block-proposers"))
	KeyCurveFactor               = BytesToBytes32([]byte("curve-factor")) // curve factor to define VTHO issuance after PoS
	KeyDelegatorContractAddress  = BytesToBytes32([]byte("delegator-contract-address"))
	// staker switches to control the pause of staker and stargate, last bit pauses the delegator contract, second-to-last bit pauses  the staker contract
	KeyStakerSwitches = BytesToBytes32([]byte("staker-switches"))

	InitialMaxBlockProposers         uint64 = 101
	InitialRewardRatio                      = big.NewInt(3e17) // 30%
	InitialValidatorRewardPercentage        = 30               // 30%
	InitialBaseGasPrice                     = big.NewInt(1e15)
	InitialProposerEndorsement              = new(big.Int).Mul(big.NewInt(1e18), big.NewInt(25000000))
	InitialCurveFactor                      = big.NewInt(76800)

	EnergyGrowthRate      = big.NewInt(5000000000) // WEI THOR per token(VET) per second. about 0.000432 THOR per token per day.
	NumberOfBlocksPerYear = big.NewInt(8640 * 365) // number of blocks per year, non leap (365 days)
)

// GetMaxBlockProposers retrieves the max block proposers parameter with fallback to initial value
// If capToInitial is true, values greater than InitialMaxBlockProposers are also capped (PoA)
func GetMaxBlockProposers(params interface {
	Get(Bytes32) (*big.Int, error)
}, capToInitial bool,
) (uint64, error) {
	mbp, err := params.Get(KeyMaxBlockProposers)
	if err != nil {
		return 0, err
	}

	if !mbp.IsUint64() {
		return 0, errors.New("max block proposers is not a uint64")
	}
	maxBlockProposers := mbp.Uint64()
	if maxBlockProposers == 0 || (capToInitial && maxBlockProposers > InitialMaxBlockProposers) {
		maxBlockProposers = InitialMaxBlockProposers
	}
	return maxBlockProposers, nil
}
