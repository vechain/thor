// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"errors"
	"io"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

type extension struct {
	BackerSignaturesRoot thor.Bytes32
	TotalBackersCount    uint64
}

type _extension extension

// EncodeRLP implements rlp.Encoder.
func (ex *extension) EncodeRLP(w io.Writer) error {
	// strictly limit extension in pre and post 193 fork stage.
	// before 193: block header must be encoded without extension
	// this is mainly for backward compatibility

	if ex.TotalBackersCount != 0 {
		return rlp.Encode(w, (*_extension)(ex))
	}
	return nil
}

// DecodeRLP implements rlp.Decoder.
func (ex *extension) DecodeRLP(s *rlp.Stream) error {
	var obj _extension
	if err := s.Decode(&obj); err != nil {
		// Error(end-of-list) means this field is not present, return default value
		// for backward compatibility
		if err == rlp.EOL {
			*ex = extension{
				emptyRoot,
				0,
			}
			return nil
		}
		return err
	}
	if obj.TotalBackersCount == 0 {
		// TotalBackersCount equals 0, bss root should be trimmed
		return errors.New("rlp: BackerSignautreRoot should be trimmed if total backers count is 0")
	}
	*ex = extension(obj)
	return nil
}
