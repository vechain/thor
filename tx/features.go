package tx

// Features bitset contains tx features.
type Features uint32

const (
	// See VIP-191 for more detail. (https://github.com/vechain/VIPs/blob/master/vips/VIP-191.md)
	delegatedMask Features = 1

	// MaxFeaturesValue max allowed features value
	MaxFeaturesValue = delegatedMask
)

// IsDelegated returns whether tx is delegated.
func (f Features) IsDelegated() bool {
	return (f & delegatedMask) == delegatedMask
}

// SetDelegated set tx delegated flag.
func (f *Features) SetDelegated(flag bool) {
	if flag {
		*f |= delegatedMask
	} else {
		*f &= (^delegatedMask)
	}
}
