package main

import (
	"os"

	"github.com/vechain/thor/v2/cmd/thor/runtime"
)

func main() {
	runtime.Start(os.Args)
}
