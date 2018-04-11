package main

import (
	"crypto/ecdsa"
	"math/rand"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/thor"
)

func initLog(lvl log15.Lvl) {
	log15.Root().SetHandler(log15.LvlFilterHandler(lvl, log15.StderrHandler))
	// set go-ethereum log lvl to Warn
	ethLogHandler := ethlog.NewGlogHandler(ethlog.StreamHandler(os.Stderr, ethlog.TerminalFormat(true)))
	ethLogHandler.Verbosity(ethlog.LvlWarn)
	ethlog.Root().SetHandler(ethLogHandler)
}

func loadNodeKey(keyFile string) (key *ecdsa.PrivateKey, err error) {
	// try to load from file
	if key, err = crypto.LoadECDSA(keyFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		return key, nil
	}

	// no such file, generate new key and write in
	key, err = crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	if err := crypto.SaveECDSA(keyFile, key); err != nil {
		return nil, err
	}
	return key, nil
}

func loadAccount(keyString string) (thor.Address, *ecdsa.PrivateKey, error) {
	if keyString != "" {
		key, err := crypto.HexToECDSA(keyString)
		if err != nil {
			return thor.Address{}, nil, err
		}
		return thor.Address(crypto.PubkeyToAddress(key.PublicKey)), key, nil
	}

	index := rand.Intn(len(genesis.DevAccounts()))
	return genesis.DevAccounts()[index].Address, genesis.DevAccounts()[index].PrivateKey, nil
}
