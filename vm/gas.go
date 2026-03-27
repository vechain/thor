// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

const (
	GasQuickStep   uint64 = 2
	GasFastestStep uint64 = 3
	GasFastStep    uint64 = 5
	GasMidStep     uint64 = 8
	GasSlowStep    uint64 = 10
	GasExtStep     uint64 = 20

	GasReturn       uint64 = 0
	GasStop         uint64 = 0
	GasContractByte uint64 = 200

	Bn256AddGasEIP1108             uint64 = 150   // Gas needed for an elliptic curve addition
	Bn256ScalarMulGasEIP1108       uint64 = 6000  // Gas needed for an elliptic curve scalar multiplication
	Bn256PairingBaseGasEIP1108     uint64 = 45000 // Base price for an elliptic curve pairing check
	Bn256PairingPerPointGasEIP1108 uint64 = 34000 // Per-point price for an elliptic curve pairing check

	// EIP-2537 BLS12-381 gas costs (Pectra/Prague)
	Bls12381G1AddGas          uint64 = 375   // Price for BLS12-381 elliptic curve G1 point addition
	Bls12381G1MulGas          uint64 = 12000 // Price for BLS12-381 elliptic curve G1 point scalar multiplication
	Bls12381G2AddGas          uint64 = 600   // Price for BLS12-381 elliptic curve G2 point addition
	Bls12381G2MulGas          uint64 = 22500 // Price for BLS12-381 elliptic curve G2 point scalar multiplication
	Bls12381PairingBaseGas    uint64 = 37700 // Base gas price for BLS12-381 elliptic curve pairing check
	Bls12381PairingPerPairGas uint64 = 32600 // Per-point pair gas price for BLS12-381 elliptic curve pairing check
	Bls12381MapG1Gas          uint64 = 5500  // Gas price for BLS12-381 mapping field element to G1 operation
	Bls12381MapG2Gas          uint64 = 23800 // Gas price for BLS12-381 mapping field element to G2 operation
)

// Bls12381G1MultiExpDiscountTable is the gas discount table for BLS12-381 G1 multi exponentiation operation.
var Bls12381G1MultiExpDiscountTable = [128]uint64{
	1000,
	949,
	848,
	797,
	764,
	750,
	738,
	728,
	719,
	712,
	705,
	698,
	692,
	687,
	682,
	677,
	673,
	669,
	665,
	661,
	658,
	654,
	651,
	648,
	645,
	642,
	640,
	637,
	635,
	632,
	630,
	627,
	625,
	623,
	621,
	619,
	617,
	615,
	613,
	611,
	609,
	608,
	606,
	604,
	603,
	601,
	599,
	598,
	596,
	595,
	593,
	592,
	591,
	589,
	588,
	586,
	585,
	584,
	582,
	581,
	580,
	579,
	577,
	576,
	575,
	574,
	573,
	572,
	570,
	569,
	568,
	567,
	566,
	565,
	564,
	563,
	562,
	561,
	560,
	559,
	558,
	557,
	556,
	555,
	554,
	553,
	552,
	551,
	550,
	549,
	548,
	547,
	547,
	546,
	545,
	544,
	543,
	542,
	541,
	540,
	540,
	539,
	538,
	537,
	536,
	536,
	535,
	534,
	533,
	532,
	532,
	531,
	530,
	529,
	528,
	528,
	527,
	526,
	525,
	525,
	524,
	523,
	522,
	522,
	521,
	520,
	520,
	519,
}

// Bls12381G2MultiExpDiscountTable is the gas discount table for BLS12-381 G2 multi exponentiation operation.
var Bls12381G2MultiExpDiscountTable = [128]uint64{
	1000,
	1000,
	923,
	884,
	855,
	832,
	812,
	796,
	782,
	770,
	759,
	749,
	740,
	732,
	724,
	717,
	711,
	704,
	699,
	693,
	688,
	683,
	679,
	674,
	670,
	666,
	663,
	659,
	655,
	652,
	649,
	646,
	643,
	640,
	637,
	634,
	632,
	629,
	627,
	624,
	622,
	620,
	618,
	615,
	613,
	611,
	609,
	607,
	606,
	604,
	602,
	600,
	598,
	597,
	595,
	593,
	592,
	590,
	589,
	587,
	586,
	584,
	583,
	582,
	580,
	579,
	578,
	576,
	575,
	574,
	573,
	571,
	570,
	569,
	568,
	567,
	566,
	565,
	563,
	562,
	561,
	560,
	559,
	558,
	557,
	556,
	555,
	554,
	553,
	552,
	552,
	551,
	550,
	549,
	548,
	547,
	546,
	545,
	545,
	544,
	543,
	542,
	541,
	541,
	540,
	539,
	538,
	537,
	537,
	536,
	535,
	535,
	534,
	533,
	532,
	532,
	531,
	530,
	530,
	529,
	528,
	528,
	527,
	526,
	526,
	525,
	524,
	524,
}

// callGas returns the actual gas cost of the call.
//
// The cost of gas was changed during the homestead price change HF. To allow for EIP150
// to be implemented. The returned gas is gas - base * 63 / 64.
func callGas(gasTable params.GasTable, availableGas, base uint64, callCost *uint256.Int) (uint64, error) {
	if gasTable.CreateBySuicide > 0 {
		availableGas = availableGas - base
		gas := availableGas - availableGas/64
		// If the bit length exceeds 64 bit we know that the newly calculated "gas" for EIP150
		// is smaller than the requested amount. Therefore we return the new gas instead
		// of returning an error.
		if !callCost.IsUint64() || gas < callCost.Uint64() {
			return gas, nil
		}
	}
	if !callCost.IsUint64() {
		return 0, ErrGasUintOverflow
	}

	return callCost.Uint64(), nil
}
