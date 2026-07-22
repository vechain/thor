// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/hex"
	"fmt"

	"github.com/vechain/thor/v2/thor"
)

// ParseBytes32Compact parses a 0x-prefixed hex string of variable length into a
// right-aligned Bytes32. Unlike thor.ParseBytes32, it accepts compact Ethereum
// encoding such as "0x0" for storage slot 0.
func ParseBytes32Compact(s string) (thor.Bytes32, error) {
	if len(s) < 2 || s[0] != '0' || (s[1] != 'x' && s[1] != 'X') {
		return thor.Bytes32{}, fmt.Errorf("invalid hex %q", s)
	}
	raw := s[2:]
	if len(raw)%2 != 0 {
		raw = "0" + raw
	}
	b, err := hex.DecodeString(raw)
	if err != nil {
		return thor.Bytes32{}, fmt.Errorf("invalid hex %q: %w", s, err)
	}
	if len(b) > 32 {
		return thor.Bytes32{}, fmt.Errorf("hex value too long for bytes32 %q", s)
	}
	var h32 thor.Bytes32
	copy(h32[32-len(b):], b)
	return h32, nil
}
