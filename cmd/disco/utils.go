// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"crypto/ecdsa"
	"os"
	"os/user"
	"path/filepath"

	"github.com/ethereum/go-ethereum/crypto"
)

func loadOrGenerateKeyFile(keyFile string) (key *ecdsa.PrivateKey, err error) {
	if !filepath.IsAbs(keyFile) {
		if keyFile, err = filepath.Abs(keyFile); err != nil {
			return nil, err
		}
	}

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

func defaultKeyFile() string {
	return filepath.Join(mustHomeDir(), ".thor-disco.key")
}

func mustHomeDir() string {
	// try to get HOME env
	if home := os.Getenv("HOME"); home != "" {
		return home
	}

	if user, err := user.Current(); err == nil {
		if user.HomeDir != "" {
			return user.HomeDir
		}
	}

	return filepath.Base(os.Args[0])
}
