// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"errors"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/thor"
)

/**
 * extension represents a data structure that follows a tail trim strategy,
 * where the last element is not considered if it is its default null value.
 */
type extension struct {
	Alpha   []byte
	COM     bool
	BaseFee *big.Int
	// ValidatorVRFProofs stores VRF proofs from validators for collective VRF selection
	// Key: validator address, Value: VRF proof
	ValidatorVRFProofs map[thor.Address][]byte
}

type _extension extension

// EncodeRLP implements rlp.Encoder.
func (ex *extension) EncodeRLP(w io.Writer) error {
	// Check if we have any non-default fields
	hasBaseFee := ex.BaseFee != nil
	hasCOM := ex.COM
	hasAlpha := len(ex.Alpha) != 0
	hasVRFProofs := len(ex.ValidatorVRFProofs) != 0

	if hasBaseFee {
		return rlp.Encode(w, (*_extension)(ex))
	}

	if hasVRFProofs {
		return rlp.Encode(w, []any{
			ex.Alpha,
			ex.COM,
			ex.ValidatorVRFProofs,
		})
	}

	if hasCOM {
		return rlp.Encode(w, []any{
			ex.Alpha,
			ex.COM,
		})
	}

	if hasAlpha {
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
			Alpha:              alpha,
			COM:                false,
			ValidatorVRFProofs: nil,
		}
		return nil
	}

	// more than one field, must have com
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
			Alpha:              alpha,
			COM:                com,
			ValidatorVRFProofs: nil,
		}
		return nil
	}

	// For three fields, check if it's BaseFee or VRFProofs
	if len(raws) == 3 {
		// Try to decode as BaseFee first (backward compatibility)
		var baseFee big.Int
		if err := rlp.DecodeBytes(raws[2], &baseFee); err == nil {
			*ex = extension{
				Alpha:              alpha,
				COM:                com,
				BaseFee:            &baseFee,
				ValidatorVRFProofs: nil,
			}
			return nil
		}

		// If not BaseFee, try to decode as VRFProofs
		var vrfProofs map[thor.Address][]byte
		if err := rlp.DecodeBytes(raws[2], &vrfProofs); err != nil {
			return err
		}

		*ex = extension{
			Alpha:              alpha,
			COM:                com,
			BaseFee:            nil,
			ValidatorVRFProofs: vrfProofs,
		}
		return nil
	}

	// For four fields, decode BaseFee and VRFProofs
	if len(raws) == 4 {
		var baseFee big.Int
		if err := rlp.DecodeBytes(raws[2], &baseFee); err != nil {
			return err
		}

		var vrfProofs map[thor.Address][]byte
		if err := rlp.DecodeBytes(raws[3], &vrfProofs); err != nil {
			return err
		}

		*ex = extension{
			Alpha:              alpha,
			COM:                com,
			BaseFee:            &baseFee,
			ValidatorVRFProofs: vrfProofs,
		}
		return nil
	}

	return nil
}
