// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"errors"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
)

/**
 * extension represents a data structure that follows a tail trim strategy,
 * where the last element is not considered if it is its default null value.
 */
type extension struct {
	Alpha    []byte
	COM      bool
	BaseFee  *big.Int
	Evidence *[][]byte
}

type _extension extension

// EncodeRLP implements rlp.Encoder.
func (ex *extension) EncodeRLP(w io.Writer) error {
	if ex.BaseFee != nil {
		return rlp.Encode(w, (*_extension)(ex))
	}

	if ex.COM {
		return rlp.Encode(w, []any{
			ex.Alpha,
			ex.COM,
		})
	}

	if len(ex.Alpha) != 0 {
		return rlp.Encode(w, []any{
			ex.Alpha,
		})
	}
	return nil
}

// DecodeRLP implements rlp.Decoder.
func (ex *extension) DecodeRLP(s *rlp.Stream) error {
	var raws []rlp.RawValue

	if err := s.Decode(&raws); err != nil {
		// Error(end-of-list) means this field is not present, return default value
		// for backward compatibility
		if err == rlp.EOL {
			*ex = extension{
				nil,
				false,
				nil,
				nil,
			}
			return nil
		}
	}

	if len(raws) == 0 || len(raws) > 4 {
		return errors.New("rlp: unexpected extension")
	}

	// alpha is always decoded
	var alpha []byte
	if err := rlp.DecodeBytes(raws[0], &alpha); err != nil {
		return err
	}

	// only alpha, make sure it's trimmed
	if len(raws) == 1 {
		if len(alpha) == 0 {
			return errors.New("rlp: extension must be trimmed")
		}

		*ex = extension{
			Alpha: alpha,
			COM:   false,
		}
		return nil
	}

	// more than one filed, must have com
	var com bool
	if err := rlp.DecodeBytes(raws[1], &com); err != nil {
		return err
	}

	// alpha and com, make sure it's trimmed
	if len(raws) == 2 {
		// COM must be trimmed if not set
		if !com {
			return errors.New("rlp: extension must be trimmed")
		}

		*ex = extension{
			Alpha: alpha,
			COM:   com,
		}
		return nil
	}

	// For three fields, decode BaseFee
	var baseFee big.Int
	if err := rlp.DecodeBytes(raws[2], &baseFee); err != nil {
		return err
	}

	var evidence [][]byte
	if err := rlp.DecodeBytes(raws[3], &evidence); err != nil {
		return err
	}

	*ex = extension{
		Alpha:    alpha,
		COM:      com,
		BaseFee:  &baseFee,
		Evidence: &evidence,
	}

	return nil
}
