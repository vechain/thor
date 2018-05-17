// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package gen

//go:generate rm -rf ./compiled/
//go:generate solc --optimize --overwrite --bin-runtime --abi -o ./compiled All.sol
//go:generate go-bindata -nometadata -pkg gen -o bindata.go compiled/
