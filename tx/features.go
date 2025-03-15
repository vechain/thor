// Copyright (c) 2019 The VeChainThor developers
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

// Features bitset contains tx features.
type Features uint32

const (
	// FeatureDelegation See VIP-191 for more detail. (https://github.com/vechain/VIPs/blob/master/vips/VIP-191.md)
	FeatureDelegation Features = 1 << 0

	// FeatureReplacement See VIP-TODO for more detail. (https://github.com/vechain/VIPs/blob/master/vips/VIP-TODO.md)
	FeatureReplacement Features = 1 << 1
)

// IsDelegated returns whether tx is delegated.
func (f Features) IsDelegated() bool {
	return (f & FeatureDelegation) != 0
}

// SetDelegated set tx delegated flag.
func (f *Features) SetDelegated(flag bool) {
	if flag {
		*f |= FeatureDelegation
	} else {
		*f &= ^FeatureDelegation
	}
}

// HasReplacement returns whether tx should be unique for origin + extra data
func (f Features) HasReplacement() bool {
	return (f & FeatureReplacement) != 0
}

func (f *Features) SetReplacement(flag bool) {
	if flag {
		*f |= FeatureReplacement
	} else {
		*f &= ^FeatureReplacement
	}
}
