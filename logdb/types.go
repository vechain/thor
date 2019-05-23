// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"math/big"

	"github.com/vechain/thor/thor"
)

//Event represents tx.Event that can be stored in db.
type Event struct {
	BlockNumber uint32
	Index       uint32
	BlockID     thor.Bytes32
	BlockTime   uint64
	TxID        thor.Bytes32
	TxOrigin    thor.Address //contract caller
	ClauseIndex uint32
	Address     thor.Address // always a contract address
	Topics      [5]*thor.Bytes32
	Data        []byte
}

//Transfer represents tx.Transfer that can be stored in db.
type Transfer struct {
	BlockNumber uint32
	Index       uint32
	BlockID     thor.Bytes32
	BlockTime   uint64
	TxID        thor.Bytes32
	TxOrigin    thor.Address
	ClauseIndex uint32
	Sender      thor.Address
	Recipient   thor.Address
	Amount      *big.Int
}

type RangeType string

const (
	Block RangeType = "block"
	Time  RangeType = "time"
)

type Order string

const (
	ASC  Order = "asc"
	DESC Order = "desc"
)

type Range struct {
	Unit RangeType
	From uint64
	To   uint64
}

type Options struct {
	Offset uint64
	Limit  uint64
}

type EventCriteria struct {
	Address *thor.Address // always a contract address
	Topics  [5]*thor.Bytes32
}

//EventFilter filter
type EventFilter struct {
	CriteriaSet []*EventCriteria
	Range       *Range
	Options     *Options
	Order       Order //default asc
}

type TransferCriteria struct {
	TxOrigin  *thor.Address //who send transaction
	Sender    *thor.Address //who transferred tokens
	Recipient *thor.Address //who recieved tokens
}

type TransferFilter struct {
	TxID        *thor.Bytes32
	CriteriaSet []*TransferCriteria
	Range       *Range
	Options     *Options
	Order       Order //default asc
}
