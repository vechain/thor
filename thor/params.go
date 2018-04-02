package thor

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/params"
)

// Constants of block chain.
const (
	BlockInterval uint64 = 10 // time interval between two consecutive blocks.

	TxGas                     uint64 = 5000
	ClauseGas                 uint64 = params.TxGas - TxGas
	ClauseGasContractCreation uint64 = params.TxGasContractCreation - TxGas

	MinGasLimit          uint64 = 1000 * 1000
	InitialGasLimit      uint64 = 10 * 1000 * 1000 // InitialGasLimit gas limit value int genesis block.
	GasLimitBoundDivisor uint64 = 1024             // from ethereum

	MaxTxWorkDelay uint32 = 30 // (unit: block) if tx delay exeeds this value, no energy can be exchanged.

	MaxBlockProposers uint64 = 101

	TolerableBlockPackingTime = 100 * time.Millisecond // the indicator to adjust target block gas limit
)

// Keys of governance params.
var (
	KeyRewardRatio         = BytesToHash([]byte("reward-ratio"))
	KeyBaseGasPrice        = BytesToHash([]byte("base-gas-price"))
	KeyProposerEndorsement = BytesToHash([]byte("proposer-endorsement"))

	InitialRewardRatio         = big.NewInt(3e17)     // 30%
	InitialBaseGasPrice        = big.NewInt(10000000) //TODO
	InitialProposerEndorsement = new(big.Int).Mul(big.NewInt(1e18), big.NewInt(250000))

	EnergyGrowthRate = big.NewInt(5000000000) // WEI THOR per token(VET) per second. about 0.000432 THOR per token per day.
)
