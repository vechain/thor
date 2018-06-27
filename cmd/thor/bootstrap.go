// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import "github.com/ethereum/go-ethereum/p2p/discover"

var bootstrapNodes = []*discover.Node{
	discover.MustParseNode("enode://6865f570268591be82d5ec3dbdea1a1833bd3fedfec21812c71f743a9521577af134de5e3ef913264321de9c084726c393e2f057b0408fdcd1d2ff144585bbad@119.28.214.38:55555"),
	discover.MustParseNode("enode://da424cb07d67400cc7e782b3d4b04c2170bddf073d665008ce3d33c332940c01881857edfb420bf1f74492d1d58becc73d20c3ef2d55d48593fa245ca2bde7a3@118.25.71.238:55555"),
	discover.MustParseNode("enode://1c8532a2c2c99be0bfbde9171122132b63a1f6e0faf4a4554e3488688da18e4502ef850c67ed2ac01f9b5792f0c4c50e41dfad0457b7ef29f1e7595f4242637b@140.143.201.56:55555"),
	discover.MustParseNode("enode://797fdd968592ca3b59a143f1aa2f152913499d4bb469f2bd5b62dfb1257707b4cb0686563fe144ee2088b1cc4f174bd72df51dbeb7ec1c5b6a8d8599c756f38b@107.150.112.22:55555"),
	discover.MustParseNode("enode://a8a83b4faac13f0a05ecd383d661a85e15e2a93fb41c4b5d00976d0bb8e35aab58a6303fe6b437124888da45017b94df8ce72f6a8bb5bcfdc7bd8df51698ad01@106.75.226.133:55555"),
}
