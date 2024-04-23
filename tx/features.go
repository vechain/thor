// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

// Features bitset contains tx features.
type Features uint32

const (
	// DelegationFeature See VIP-191 for more detail. (https://github.com/vechain/VIPs/blob/master/vips/VIP-191.md)
	DelegationFeature Features = 1
)

// IsDelegated returns whether tx is delegated.
func (f Features) IsDelegated() bool {
	return (f & DelegationFeature) == DelegationFeature
}

// SetDelegated set tx delegated flag.
func (f *Features) SetDelegated(flag bool) {
	if flag {
		*f |= DelegationFeature
	} else {
		*f &= (^DelegationFeature)
	}
}
