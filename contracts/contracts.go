package contracts

import "github.com/vechain/thor/thor"

// All genesis contracts
var (
	Authority = mustLoad(
		thor.BytesToAddress([]byte("aut")),
		"compiled/Authority.abi",
		"compiled/Authority.bin-runtime")

	Energy = mustLoad(
		thor.BytesToAddress([]byte("eng")),
		"compiled/Energy.abi",
		"compiled/Energy.bin-runtime")
)
