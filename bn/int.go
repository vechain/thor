package bn

import (
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
)

var big0 = new(big.Int)

// Int wraps big.Int.
// It can be used as a value without state sharing.
type Int struct {
	value *big.Int
}

// FromBig create a bn.Int object from big.Int.
func FromBig(bi *big.Int) Int {
	i := Int{}
	i.SetBig(bi)
	return i
}

// ToBig convert to big.Int.
func (i Int) ToBig() *big.Int {
	if i.value == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(i.value)
}

// SetBig set big.Int.
func (i *Int) SetBig(bi *big.Int) {
	if bi.Sign() == 0 {
		i.value = nil
		return
	}
	i.value = new(big.Int).Set(bi)
}

// IsZero returns true if bn.Int presents a zero value.
func (i Int) IsZero() bool {
	return i.value == nil || i.value.Sign() == 0
}

// Cmp compares with another bn.Int.
// Returns:
//   -1 if i <  other
//    0 if i == other
//   +1 if i >  other
//
func (i Int) Cmp(other Int) int {
	if i.value == nil {
		if other.value == nil {
			return 0
		}
		return -other.value.Sign()
	}

	if other.value == nil {
		return i.value.Sign()
	}
	return i.value.Cmp(other.value)
}

// CmpBig compares with big.Int value.
// Returns:
//   -1 if i <  bi
//    0 if i == bi
//   +1 if i >  bi
//
func (i Int) CmpBig(bi *big.Int) int {
	if i.value == nil {
		return -bi.Sign()
	}
	return i.value.Cmp(bi)
}

// EncodeRLP implements rlp.Encoder.
func (i Int) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, i.value)
}

// DecodeRLP implements rlp.Decoder.
func (i *Int) DecodeRLP(s *rlp.Stream) error {
	return s.Decode(&i.value)
}

// String implements Stringer.
func (i Int) String() string {
	if i.value == nil {
		return big0.String()
	}
	return i.value.String()
}

// Format see big.Int.Format.
func (i Int) Format(s fmt.State, ch rune) {
	if i.value == nil {
		big0.Format(s, ch)
		return
	}
	i.value.Format(s, ch)
}

// MarshalText implements the encoding.TextMarshaler interface.
func (i Int) MarshalText() (text []byte, err error) {
	if i.value == nil {
		return big0.MarshalText()
	}
	return i.value.MarshalText()
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (i *Int) UnmarshalText(text []byte) error {
	bi := new(big.Int)
	if err := bi.UnmarshalText(text); err != nil {
		return err
	}
	if bi.Sign() == 0 {
		i.value = nil
	} else {
		i.value = bi
	}
	return nil
}

// MarshalJSON implements the json.Marshaler interface.
func (i Int) MarshalJSON() ([]byte, error) {
	if i.value == nil {
		return big0.MarshalJSON()
	}
	return i.value.MarshalJSON()
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (i *Int) UnmarshalJSON(text []byte) error {
	bi := new(big.Int)
	if err := bi.UnmarshalJSON(text); err != nil {
		return err
	}
	if bi.Sign() == 0 {
		i.value = nil
	} else {
		i.value = bi
	}
	return nil
}
