package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/vechain/thor/vrf"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("command: vrfkey <DIR>")
		return
	}

	dir := os.Args[1]

	if dir[len(dir)-1:] != "/" {
		dir += "/"
	}

	file := dir + "master.key"
	key, err := ioutil.ReadFile(file)
	if err != nil {
		panic(errors.WithMessage(err, "master.key not found"))
	}

	sk, err := crypto.HexToECDSA(string(key))
	if err != nil {
		panic(errors.WithMessage(err, "invalid private key"))
	}

	vrfpk, vrfsk := vrf.GenKeyPairFromSeed(sk.D.Bytes())

	fmt.Printf("sk: %x\n", vrfsk[:])
	fmt.Printf("pk: %x\n", vrfpk[:])

	return
}
