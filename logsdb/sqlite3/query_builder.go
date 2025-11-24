// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package sqlite3

import (
	"fmt"

	"github.com/vechain/thor/v2/logsdb"
)

const (
	refIDQuery = "(SELECT id FROM ref WHERE data=?)"
)

func eventCriteriaToWhereCondition(c *logsdb.EventCriteria) (cond string, args []any) {
	cond = "1"
	if c.Address != nil {
		cond += " AND address = " + refIDQuery
		args = append(args, c.Address.Bytes())
	}
	for i, topic := range c.Topics {
		if topic != nil {
			cond += fmt.Sprintf(" AND topic%v = ", i) + refIDQuery
			args = append(args, removeLeadingZeros(topic.Bytes()))
		}
	}
	return
}

func transferCriteriaToWhereCondition(c *logsdb.TransferCriteria) (cond string, args []any) {
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

func removeLeadingZeros(bytes []byte) []byte {
	i := 0
	// increase i until it reaches the first non-zero byte
	for ; i < len(bytes) && bytes[i] == 0; i++ {
	}
	// ensure at least 1 byte exists
	if i == len(bytes) {
		return []byte{0}
	}
	return bytes[i:]
}
