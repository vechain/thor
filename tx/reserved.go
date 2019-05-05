package tx

import (
	"bytes"
	"errors"
	"io"

	"github.com/ethereum/go-ethereum/rlp"
)

type reserved struct {
	Features Features
}

func (r *reserved) EncodeRLP(w io.Writer) error {
	fields := []interface{}{r.Features}
	var raws []rlp.RawValue
	for _, v := range fields {
		raw, err := rlp.EncodeToBytes(v)
		if err != nil {
			return err
		}
		raws = append(raws, raw)
	}

	for i := len(raws); i > 0; {
		last := raws[i-1]
		if bytes.Equal(last, rlp.EmptyList) || bytes.Equal(last, rlp.EmptyString) {
			raws = raws[:i-1] // pop the last empty value to trim
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

	if i := len(raws); i > 0 {
		last := raws[i-1]
		if bytes.Equal(last, rlp.EmptyList) || bytes.Equal(last, rlp.EmptyString) {
			return errors.New("tx reserved field not trimmed")
		}
	}

	fields := []interface{}{&r.Features}
	if len(raws) > len(fields) {
		return errors.New("tx reserved field incompatible")
	}

	for i, raw := range raws {
		if err := rlp.DecodeBytes(raw, fields[i]); err != nil {
			return err
		}
	}
	return nil
}
