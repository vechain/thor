package subscriptions

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type SubscriptionBlock struct {
	Number       uint32         `json:"number"`
	ID           thor.Bytes32   `json:"id"`
	Size         uint32         `json:"size"`
	ParentID     thor.Bytes32   `json:"parentID"`
	Timestamp    uint64         `json:"timestamp"`
	GasLimit     uint64         `json:"gasLimit"`
	Beneficiary  thor.Address   `json:"beneficiary"`
	GasUsed      uint64         `json:"gasUsed"`
	TotalScore   uint64         `json:"totalScore"`
	TxsRoot      thor.Bytes32   `json:"txsRoot"`
	StateRoot    thor.Bytes32   `json:"stateRoot"`
	ReceiptsRoot thor.Bytes32   `json:"receiptsRoot"`
	Signer       thor.Address   `json:"signer"`
	Transactions []thor.Bytes32 `json:"transactions"`
	Obsolete     bool           `json:"obsolete"`
}

func convertBlock(b *chain.Block) (*SubscriptionBlock, error) {
	if b == nil {
		return nil, nil
	}
	header := b.Header()
	signer, err := header.Signer()
	if err != nil {
		return nil, err
	}

	txs := b.Transactions()
	txIds := make([]thor.Bytes32, len(txs))
	for i, tx := range txs {
		txIds[i] = tx.ID()
	}
	return &SubscriptionBlock{
		Number:       header.Number(),
		ID:           header.ID(),
		ParentID:     header.ParentID(),
		Timestamp:    header.Timestamp(),
		TotalScore:   header.TotalScore(),
		GasLimit:     header.GasLimit(),
		GasUsed:      header.GasUsed(),
		Beneficiary:  header.Beneficiary(),
		Signer:       signer,
		Size:         uint32(b.Size()),
		StateRoot:    header.StateRoot(),
		ReceiptsRoot: header.ReceiptsRoot(),
		TxsRoot:      header.TxsRoot(),
		Transactions: txIds,
		Obsolete:     b.Obsolete,
	}, nil
}

type LogMeta struct {
	BlockID        thor.Bytes32 `json:"blockID"`
	BlockNumber    uint32       `json:"blockNumber"`
	BlockTimestamp uint64       `json:"blockTimestamp"`
	TxID           thor.Bytes32 `json:"txID"`
	TxOrigin       thor.Address `json:"txOrigin"`
}

type SubscriptionTransfer struct {
	Sender    thor.Address          `json:"sender"`
	Recipient thor.Address          `json:"recipient"`
	Amount    *math.HexOrDecimal256 `json:"amount"`
	Meta      LogMeta               `json:"meta"`
	Obsolete  bool                  `json:"obsolete"`
}

func convertTransfer(header *block.Header, origin thor.Address, tx *tx.Transaction, transfer *tx.Transfer, obsolete bool) (*SubscriptionTransfer, error) {
	signer, err := tx.Signer()
	if err != nil {
		return nil, err
	}
	v := math.HexOrDecimal256(*transfer.Amount)
	return &SubscriptionTransfer{
		Sender:    transfer.Sender,
		Recipient: transfer.Recipient,
		Amount:    &v,
		Meta: LogMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
			TxID:           tx.ID(),
			TxOrigin:       signer,
		},
		Obsolete: obsolete,
	}, nil
}

type SubscriptionEvent struct {
	Address  thor.Address   `json:"address"`
	Topics   []thor.Bytes32 `json:"topics"`
	Data     string         `json:"data"`
	Meta     LogMeta        `json:"meta"`
	Obsolete bool           `json:"obsolete"`
}

func convertEvent(header *block.Header, origin thor.Address, tx *tx.Transaction, event *tx.Event, obsolete bool) (*SubscriptionEvent, error) {
	signer, err := tx.Signer()
	if err != nil {
		return nil, err
	}
	return &SubscriptionEvent{
		Address: event.Address,
		Data:    hexutil.Encode(event.Data),
		Meta: LogMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
			TxID:           tx.ID(),
			TxOrigin:       signer,
		},
		Topics:   event.Topics,
		Obsolete: obsolete,
	}, nil
}

// EventFilter contains options for contract event filtering.
type EventFilter struct {
	Address *thor.Address // restricts matches to events created by specific contracts
	Topic0  *thor.Bytes32
	Topic1  *thor.Bytes32
	Topic2  *thor.Bytes32
	Topic3  *thor.Bytes32
	Topic4  *thor.Bytes32
}

func (ef *EventFilter) match(event *tx.Event) bool {
	if (ef.Address != nil) && (*ef.Address != event.Address) {
		return false
	}

	matchTopic := func(topic *thor.Bytes32, index int) bool {
		if topic != nil {
			if len(event.Topics) <= index {
				return false
			}

			if *topic != event.Topics[index] {
				return false
			}
		}
		return true
	}

	return matchTopic(ef.Topic0, 0) &&
		matchTopic(ef.Topic1, 1) &&
		matchTopic(ef.Topic2, 2) &&
		matchTopic(ef.Topic3, 3) &&
		matchTopic(ef.Topic4, 4)
}

// TransferFilter contains options for contract transfer filtering.
type TransferFilter struct {
	TxOrigin  *thor.Address // who send transaction
	Sender    *thor.Address // who transferred tokens
	Recipient *thor.Address // who recieved tokens
}

func (tf *TransferFilter) match(transfer *tx.Transfer, origin thor.Address) bool {
	if (tf.TxOrigin != nil) && (*tf.TxOrigin != origin) {
		return false
	}

	if (tf.Sender != nil) && (*tf.Sender != transfer.Sender) {
		return false
	}

	if (tf.Recipient != nil) && (*tf.Recipient != transfer.Recipient) {
		return false
	}
	return true
}

type Output struct {
	*tx.Output
	header   *block.Header
	origin   thor.Address
	tx       *tx.Transaction
	obsolete bool
}

func extractOutputs(chain *chain.Chain, blks []*chain.Block) ([]*Output, error) {
	result := []*Output{}
	for _, blk := range blks {
		receipts, err := chain.GetBlockReceipts(blk.Header().ID())
		if err != nil {
			return nil, err
		}

		for i, receipt := range receipts {
			tx := blk.Transactions()[i]
			origin, err := tx.Signer()
			if err != nil {
				return nil, err
			}

			for _, output := range receipt.Outputs {
				result = append(result, &Output{output, blk.Header(), origin, tx, blk.Obsolete})
			}
		}
	}
	return result, nil
}
