// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package gen

//go:generate rm -rf ./compiled/
//go:generate solc --optimize-runs 200 --overwrite --bin-runtime --abi -o ./compiled authority.sol energy.sol executor.sol extension.sol extension-v2.sol measure.sol params.sol prototype.sol
//go:generate go-bindata -nometadata -ignore=_ -pkg gen -o bindata.go compiled/
