// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

type Master struct {
	PrivateKey  *ecdsa.PrivateKey
	Beneficiary *thor.Address

	VrfPrivateKey *vrf.PrivateKey
}

func (m *Master) deriveVrfPrivateKey() {
	// pass the master private key to derive the VRF private key
	// VRF private key [:32] = master private key
	// VRF private key [32:] = VRF public key
	_, m.VrfPrivateKey = vrf.GenKeyPairFromSeed(m.PrivateKey.D.Bytes())
}

func (m *Master) Address() thor.Address {
	return thor.Address(crypto.PubkeyToAddress(m.PrivateKey.PublicKey))
}
