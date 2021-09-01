// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"errors"
	"io"

	"github.com/ethereum/go-ethereum/rlp"
)

type extension struct {
	Alpha []byte
}

type _extension extension

// EncodeRLP implements rlp.Encoder.
func (ex *extension) EncodeRLP(w io.Writer) error {
	// trim extension before VIP214
	// this is mainly for backward compatibility
	if len(ex.Alpha) == 0 {
		return nil
	}
	return rlp.Encode(w, (*_extension)(ex))
}

// DecodeRLP implements rlp.Decoder.
func (ex *extension) DecodeRLP(s *rlp.Stream) error {
	var obj _extension
	if err := s.Decode(&obj); err != nil {
		// Error(end-of-list) means this field is not present, return default value
		// for backward compatibility
		if err == rlp.EOL {
			*ex = extension{
				nil,
			}
			return nil
		}
		return err
	}
	if len(obj.Alpha) == 0 {
		return errors.New("rlp: extension must be trimmed")
	}
	*ex = extension(obj)
	return nil
}
