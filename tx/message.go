package tx

import (
	"math/big"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// Message is fully compliant with vm.Message, which describes a clause with tx context.
type Message interface {
	From() acc.Address
	To() *acc.Address
	GasPrice() *big.Int
	GasLimit() *big.Int
	Value() *big.Int
	Data() []byte

	// returns hash of tx which generated this message.
	TransactionHash() cry.Hash
	// create a new message instance with new gas limit.
	WithGasLimit(gl *big.Int) Message
}

// message implements tx.Message.
// It's immutable.
type message struct {
	tx          *Transaction
	clauseIndex int
	from        acc.Address
	gasLimit    *big.Int
}

// newMessage create a new instance of message.
func newMessage(tx *Transaction, clauseIndex int, from acc.Address) Message {
	return &message{
		tx,
		clauseIndex,
		from,
		tx.GasLimit(),
	}
}

func (m *message) From() acc.Address {
	return m.from
}

func (m *message) To() *acc.Address {
	to := m.tx.body.Clauses[m.clauseIndex].To
	if to != nil {
		cpy := *to
		return &cpy
	}
	return nil
}

func (m *message) GasPrice() *big.Int {
	return m.tx.GasPrice()
}

func (m *message) GasLimit() *big.Int {
	// here returns gas limit in message, not of tx.
	// since we may create new message with different gas limit.
	return new(big.Int).Set(m.gasLimit)
}

func (m *message) Value() *big.Int {
	value := m.tx.body.Clauses[m.clauseIndex].Value
	return new(big.Int).Set(value)
}

func (m *message) Data() []byte {
	data := m.tx.body.Clauses[m.clauseIndex].Data
	return append([]byte(nil), data...)
}

func (m message) WithGasLimit(gl *big.Int) Message {
	m.gasLimit = new(big.Int).Set(gl)
	return &m
}

func (m *message) TransactionHash() cry.Hash {
	return m.tx.Hash()
}
