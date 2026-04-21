// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import "github.com/vechain/thor/v2/metrics"

// bucketEVMCallDepth covers the EVM's practical call-depth range.
// The protocol hard-cap is 1024; real-world depths cluster at 0–10.
var bucketEVMCallDepth = []int64{0, 1, 2, 3, 5, 10, 20, 50, 100, 1024}

var (
	metricChainIDOpcodeCount     = metrics.LazyLoadCounterVec("vm_chainid_opcode_count", []string{"call_type"})
	metricChainIDOpcodeCallDepth = metrics.LazyLoadHistogram("vm_chainid_opcode_call_depth", bucketEVMCallDepth)
)
