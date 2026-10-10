// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/thor"
)

func ConvertCallResultWithInputGas(vo *runtime.Output, inputGas uint64) *api.CallResult {
	gasUsed := inputGas - vo.LeftOverGas
	var (
		vmError  string
		reverted bool
	)

	if vo.VMErr != nil {
		reverted = true
		vmError = vo.VMErr.Error()
	}

	events := make([]*api.Event, len(vo.Events))
	transfers := make([]*api.Transfer, len(vo.Transfers))

	for j, txEvent := range vo.Events {
		event := &api.Event{
			Address: txEvent.Address,
			Data:    hexutil.Encode(txEvent.Data),
		}
		event.Topics = make([]thor.Bytes32, len(txEvent.Topics))
		copy(event.Topics, txEvent.Topics)
		events[j] = event
	}
	for j, txTransfer := range vo.Transfers {
		transfer := &api.Transfer{
			Sender:    txTransfer.Sender,
			Recipient: txTransfer.Recipient,
			Amount:    (*math.HexOrDecimal256)(txTransfer.Amount),
		}
		transfers[j] = transfer
	}

	return &api.CallResult{
		Data:      hexutil.Encode(vo.Data),
		Events:    events,
		Transfers: transfers,
		GasUsed:   gasUsed,
		Reverted:  reverted,
		VMError:   vmError,
	}
}
