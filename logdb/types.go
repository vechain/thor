// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"fmt"
	"math/big"
	"strings"

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
	TxOrigin    thor.Address //contract caller
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

func (c *EventCriteria) toWhereCondition(query string) (cond string, args []any, adjustedQuery string) {
	builder := strings.Builder{}
	adjustedQuery = query
	if c.Address != nil {
		builder.WriteString(" r3.data = ?")
		args = append(args, c.Address.Bytes())
		adjustedQuery = strings.Replace(adjustedQuery, "LEFT JOIN ref r3 ON e.address", "INNER JOIN ref r3 ON e.address", 1)
	}
	for i, topic := range c.Topics {
		if topic != nil {
			if builder.String() != "" {
				builder.WriteString(" AND ")
			}
			builder.WriteString(fmt.Sprintf(" r%v.data = ?", i+4))
			args = append(args, removeLeadingZeros(topic.Bytes()))
			adjustedQuery = strings.Replace(adjustedQuery, fmt.Sprintf("LEFT JOIN ref r%v", i+4), fmt.Sprintf("INNER JOIN ref r%v", i+4), 1)
		}
	}
	return builder.String(), args, adjustedQuery
}

// EventFilter filter
type EventFilter struct {
	CriteriaSet []*EventCriteria
	Range       *Range
	Options     *Options
	Order       Order //default asc
}

type TransferCriteria struct {
	TxOrigin  *thor.Address //who send transaction
	Sender    *thor.Address //who transferred tokens
	Recipient *thor.Address //who received tokens
}

func (c *TransferCriteria) toWhereCondition() (cond string, args []any) {
	builder := strings.Builder{}
	if c.TxOrigin != nil {
		builder.WriteString(" r2.data = ?")
		args = append(args, c.TxOrigin.Bytes())
	}
	if c.Sender != nil {
		if builder.String() != "" {
			builder.WriteString(" AND ")
		}
		builder.WriteString(" r3.data = ?")
		args = append(args, c.Sender.Bytes())
	}
	if c.Recipient != nil {
		if builder.String() != "" {
			builder.WriteString(" AND ")
		}
		builder.WriteString(" r4.data = ?")
		args = append(args, c.Recipient.Bytes())
	}
	return builder.String(), args
}

type TransferFilter struct {
	CriteriaSet []*TransferCriteria
	Range       *Range
	Options     *Options
	Order       Order //default asc
}
