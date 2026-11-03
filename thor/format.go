// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"crypto/sha256"
	"fmt"
	"math/big"
)

// BigIntToString formats a big.Int for logs and error messages. Values wider
// than 256 bits are rendered as a bounded summary instead of the full decimal
// expansion.
func BigIntToString(v *big.Int) string {
	if v == nil {
		return "<nil>"
	}
	const maxBits = 256
	if v.BitLen() <= maxBits {
		return v.String()
	}
	b := v.Bytes()
	prefix := b
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("<oversized bigint bitlen=%d prefix=0x%x sha256=%x>", v.BitLen(), prefix, sum)
}
