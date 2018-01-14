package thor

import (
	"github.com/ethereum/go-ethereum/params"
)

// Constants of block chain.
const (
	BlockInterval             uint64 = 10 // time interval between two consecutive blocks.
	ClauseGas                 uint64 = params.TxGas * 2 / 3
	ClauseGasContractCreation uint64 = params.TxGasContractCreation * 2 / 3
	InitialGasLimit           uint64 = 10 * 1000 * 1000 // InitialGasLimit gas limit value int genesis block.
)
