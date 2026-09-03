// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package datagen

import (
	"crypto/rand"

	"github.com/vechain/thor/v2/thor"
)

func RandAddress() (addr thor.Address) {
	rand.Read(addr[:])
	return
}

func RandAddressPtr() *thor.Address {
	addr := RandAddress()
	return &addr
}

func RandAddresses(n int) (addrs []thor.Address) {
	addrs = make([]thor.Address, n)
	for i := range addrs {
		addrs[i] = RandAddress()
	}
	return
}

func RandAddressesPtr(n int) (addrs []*thor.Address) {
	addrs = make([]*thor.Address, n)
	for i := range addrs {
		addrs[i] = RandAddressPtr()
	}
	return
}
