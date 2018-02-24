package thor

import (
	"math/big"

	"github.com/ethereum/go-ethereum/params"
)

// Constants of block chain.
const (
	BlockInterval             uint64 = 10 // time interval between two consecutive blocks.
	ClauseGas                 uint64 = params.TxGas * 2 / 3
	ClauseGasContractCreation uint64 = params.TxGasContractCreation * 2 / 3

	MinGasLimit          uint64 = 1000 * 1000
	InitialGasLimit      uint64 = 10 * 1000 * 1000 // InitialGasLimit gas limit value int genesis block.
	GasLimitBoundDivisor uint64 = 1024             // from ethereum

	MaxTxWorkDelay uint64 = 60 * 5 // (unit: second) if tx delay exeeds this value, no energy can be exchanged.

	MaxBlockProposers uint64 = 101
)

// Keys of governance params.
var (
	KeyRewardRatio  = BytesToHash([]byte("reward-ratio"))
	KeyBaseGasPrice = BytesToHash([]byte("base-gas-price"))

	InitialRewardRatio      = big.NewInt(3e17)       // 30%
	InitialBaseGasPrice     = big.NewInt(10000000)   //TODO
	InitialEnergyGrowthRate = big.NewInt(5000000000) // WEI THOR per token(VET) per second. about 0.000432 THOR per token per day.
)
