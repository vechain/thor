package tx

import (
	"errors"
	"io"

	"github.com/ethereum/go-ethereum/rlp"
)

type reserved struct {
	Features Features
}

func (r *reserved) EncodeRLP(w io.Writer) error {
	if r.Features == 0 {
		w.Write(rlp.EmptyList)
		return nil
	}

	return rlp.Encode(w, r)
}

func (r *reserved) DecodeRLP(s *rlp.Stream) error {
	var raws []rlp.RawValue
	if err := s.Decode(&raws); err != nil {
		return err
	}

	switch len(raws) {
	case 0:
		r.Features = 0
		return nil
	case 1:
		var feat Features
		if err := rlp.DecodeBytes(raws[0], &feat); err != nil {
			return err
		}
		if feat == 0 {
			return errors.New("tx reserved field not trimmed")
		}
		r.Features = feat
		return nil
	default:
		return errors.New("tx reserved field incompatible")
	}
}
