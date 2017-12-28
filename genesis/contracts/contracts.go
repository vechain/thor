package contracts

import "github.com/vechain/thor/acc"

// All genesis contracts
var (
	Authority = mustLoad(
		acc.BytesToAddress([]byte("au")),
		"compiled/Authority.abi",
		"compiled/Authority.bin-runtime")

	Energy = mustLoad(
		acc.BytesToAddress([]byte("en")),
		"compiled/Energy.abi",
		"compiled/Energy.bin-runtime")
)
