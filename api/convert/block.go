// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

// ConvertBlockBase converts a block header into its wire base fields.
func ConvertBlockBase(header *block.Header, size uint32, signer thor.Address) dto.BlockBase {
	return dto.BlockBase{
		Number:        header.Number(),
		ID:            header.ID(),
		ParentID:      header.ParentID(),
		Timestamp:     header.Timestamp(),
		TotalScore:    header.TotalScore(),
		GasLimit:      header.GasLimit(),
		GasUsed:       header.GasUsed(),
		Beneficiary:   header.Beneficiary(),
		Signer:        signer,
		Size:          size,
		StateRoot:     header.StateRoot(),
		ReceiptsRoot:  header.ReceiptsRoot(),
		TxsRoot:       header.TxsRoot(),
		TxsFeatures:   uint32(header.TxsFeatures()),
		COM:           header.COM(),
		BaseFeePerGas: (*math.HexOrDecimal256)(header.BaseFee()),
	}
}
