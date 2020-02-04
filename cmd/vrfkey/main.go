package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/vrf"
)

func main() {
	dir := os.Args[1]

	if len(dir) == 0 {
		panic("master key directory required")
	}

	if dir[len(dir)-1:] != "/" {
		dir += "/"
	}

	file := dir + "master.key"
	key, err := ioutil.ReadFile(file)
	if err != nil {
		panic(err)
	}

	sk, err := crypto.HexToECDSA(string(key))
	if err != nil {
		panic(err)
	}

	vrfpk, vrfsk := vrf.GenKeyPairFromSeed(sk.D.Bytes())

	fmt.Printf("sk: %x\n", vrfsk[:])
	fmt.Printf("pk: %x\n", vrfpk[:])

	return
}
