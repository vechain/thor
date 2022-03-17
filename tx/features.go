package tx

// Features bitset contains tx features.
type Features uint32

const (
	// DelegationFeature See VIP-191 for more detail. (https://github.com/vechain/VIPs/blob/master/vips/VIP-191.md)
	DelegationFeature Features = 1
	EthTxnFeature     Features = 2
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

// IsEthTransaction returns whether tx is delegated.
func (f Features) IsEthTransaction() bool {
	return (f & EthTxnFeature) == EthTxnFeature
}

// SetDelegated set tx delegated flag.
func (f *Features) SetEthTransaction(flag bool) {
	if flag {
		*f |= EthTxnFeature
	} else {
		*f &= (^EthTxnFeature)
	}
}
