// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import "github.com/ethereum/go-ethereum/p2p/discover"

var bootstrapNodes = []*discover.Node{
	discover.MustParseNode("enode://797fdd968592ca3b59a143f1aa2f152913499d4bb469f2bd5b62dfb1257707b4cb0686563fe144ee2088b1cc4f174bd72df51dbeb7ec1c5b6a8d8599c756f38b@107.150.112.22:55555"),
	discover.MustParseNode("enode://a8a83b4faac13f0a05ecd383d661a85e15e2a93fb41c4b5d00976d0bb8e35aab58a6303fe6b437124888da45017b94df8ce72f6a8bb5bcfdc7bd8df51698ad01@106.75.226.133:55555"),
	discover.MustParseNode("enode://e42edaa9bee0c324ffd63600d435dc22b88f777aaacadcdb257110e81d57fb4e796b9277ff81f367d578ff9521525a89b78e21332ba7081a7322899a9f352837@106.75.226.228:55555"),
	discover.MustParseNode("enode://3eae6740af6180bb015309f7a07ff7405d6f1f9f1e5a9f2fabbd36b0c00b862521e63ff23573ffdb9035f2237c26513cb9f02454f9ada993e60b99ffc187bb54@107.150.112.21:55555"),
}
