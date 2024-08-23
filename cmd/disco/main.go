package main

import (
	"os"

	"github.com/vechain/thor/v2/cmd/disco/runtime"
)

func main() {
	runtime.Start(os.Args)
}
