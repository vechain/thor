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

type backerSignaturesRoot struct {
	Root              thor.Bytes32
	TotalBackersCount uint64
}

type _bssRoot backerSignaturesRoot

// EncodeRLP implements rlp.Encoder.
func (br *backerSignaturesRoot) EncodeRLP(w io.Writer) error {
	// strictly limit backer signatures root in pre and post 193 fork stage.
	// before 193: block header must be encoded without BackerSignatureRoot
	// this is mainly for backward compatibility

	if br.TotalBackersCount != 0 {
		return rlp.Encode(w, (*_bssRoot)(br))
	}
	return nil
}

// DecodeRLP implements rlp.Decoder.
func (br *backerSignaturesRoot) DecodeRLP(s *rlp.Stream) error {
	var obj _bssRoot
	if err := s.Decode(&obj); err != nil {
		// Error(end-of-list) means this field is not present, return default value
		// for backward compatibility
		if err == rlp.EOL {
			*br = backerSignaturesRoot{
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
	*br = backerSignaturesRoot(obj)
	return nil
}
