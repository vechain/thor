package tx

import (
	"bytes"
	"errors"
	"io"

	"github.com/ethereum/go-ethereum/rlp"
)

type reserved struct {
	Features Features
	Unused   []rlp.RawValue
}

func (r *reserved) EncodeRLP(w io.Writer) error {
	featuresRaw, _ := rlp.EncodeToBytes(r.Features)
	raws := append([]rlp.RawValue{featuresRaw}, r.Unused...)

	for i := len(raws) - 1; i >= 0; i-- {
		if isEmptyRLPRaw(raws[i]) {
			raws = raws[:i] // pop the last empty value to trim
		} else {
			break
		}
	}

	return rlp.Encode(w, raws)
}

func (r *reserved) DecodeRLP(s *rlp.Stream) error {
	var raws []rlp.RawValue
	if err := s.Decode(&raws); err != nil {
		return err
	}

	if len := len(raws); len > 0 {
		if isEmptyRLPRaw(raws[len-1]) {
			return errors.New("invalid reserved fields: not trimmed")
		}

		var feat Features
		if err := rlp.DecodeBytes(raws[0], &feat); err != nil {
			return err
		}
		r.Features = feat
		if len > 1 {
			r.Unused = raws[1:]
		} else {
			r.Unused = nil
		}
	} else {
		r.Features = 0
		r.Unused = nil
	}
	return nil
}

func isEmptyRLPRaw(value rlp.RawValue) bool {
	return len(value) == 0 ||
		bytes.Equal(value, rlp.EmptyList) ||
		bytes.Equal(value, rlp.EmptyString)
}
