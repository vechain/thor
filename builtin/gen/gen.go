// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package gen

//go:generate rm -rf ./compiled/
//go:generate docker run -v ./:/solidity ethereum/solc:0.4.24 --optimize-runs 200 --overwrite --bin-runtime --abi -o /solidity/compiled authority.sol energy.sol executor.sol extension.sol extension-v2.sol measure.sol params.sol prototype.sol
//go:generate go run github.com/go-bindata/go-bindata/go-bindata@v1.0.0 -nometadata -ignore=_ -pkg gen -o bindata.go compiled/
//go:generate go fmt
