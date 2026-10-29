// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"fmt"
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

// Event represents tx.Event that can be stored in db.
type Event struct {
	BlockNumber uint32
	LogIndex    uint32
	BlockID     thor.Bytes32
	BlockTime   uint64
	TxID        thor.Bytes32
	TxIndex     uint32
	TxOrigin    thor.Address // contract caller
	ClauseIndex uint32
	Address     thor.Address // always a contract address
	Topics      [5]*thor.Bytes32
	Data        []byte
}

// Transfer represents tx.Transfer that can be stored in db.
type Transfer struct {
	BlockNumber uint32
	LogIndex    uint32
	BlockID     thor.Bytes32
	BlockTime   uint64
	TxID        thor.Bytes32
	TxIndex     uint32
	TxOrigin    thor.Address
	ClauseIndex uint32
	Sender      thor.Address
	Recipient   thor.Address
	Amount      *big.Int
}

type Order string

const (
	ASC  Order = "asc"
	DESC Order = "desc"
)

type Range struct {
	From uint32
	To   uint32
}

type Options struct {
	Offset uint64
	Limit  uint64
}

type EventCriteria struct {
	Address *thor.Address // always a contract address
	Topics  [5]*thor.Bytes32
}

// maxScanBlocks bounds the block span over which address-less criteria are evaluated by a seq
// range scan rather than a topic index; past this width the scan stops paying off for sparse topics.
const maxScanBlocks = 100_000

// seqBounded reports whether an index lookup for the criterion can be bounded by the seq range.
// Every event index except event_i0 has address between its topic columns and seq, so without an
// address the lookup reads every matching entry chain-wide before the seq filter applies.
func (c *EventCriteria) seqBounded() bool {
	return c.Address != nil
}

// toWhereCondition renders the criterion. With rangeScan set, topics are written as +topicN so
// SQLite cannot use them for an index lookup and falls back to the seq range.
func (c *EventCriteria) toWhereCondition(rangeScan bool) (cond string, args []any) {
	cond = "1"
	if c.Address != nil {
		cond += " AND address = " + refIDQuery
		args = append(args, c.Address.Bytes())
	}
	for i, topic := range c.Topics {
		if topic != nil {
			col := fmt.Sprintf("topic%v", i)
			if rangeScan {
				col = "+" + col
			}
			cond += " AND " + col + " = " + refIDQuery
			args = append(args, removeLeadingZeros(topic.Bytes()))
		}
	}
	return
}

// EventFilter filter
type EventFilter struct {
	CriteriaSet []*EventCriteria
	Range       *Range
	Options     *Options
	Order       Order // default asc
}

type TransferCriteria struct {
	TxOrigin  *thor.Address // who send transaction
	Sender    *thor.Address // who transferred tokens
	Recipient *thor.Address // who received tokens
}

func (c *TransferCriteria) toWhereCondition() (cond string, args []any) {
	cond = "1"
	if c.TxOrigin != nil {
		cond += " AND txOrigin = " + refIDQuery
		args = append(args, c.TxOrigin.Bytes())
	}
	if c.Sender != nil {
		cond += " AND sender = " + refIDQuery
		args = append(args, c.Sender.Bytes())
	}
	if c.Recipient != nil {
		cond += " AND recipient = " + refIDQuery
		args = append(args, c.Recipient.Bytes())
	}
	return
}

type TransferFilter struct {
	CriteriaSet []*TransferCriteria
	Range       *Range
	Options     *Options
	Order       Order // default asc
}
