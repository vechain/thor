// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestConvertBlockWithBadSignature(t *testing.T) {
	// Arrange
	badSig := bytes.Repeat([]byte{0xf}, 65)

	b := new(block.Builder).
		Build().
		WithSignature(badSig[:])

	extendedBlock := &chain.ExtendedBlock{Block: b, Obsolete: false}

	// Act
	blockMessage, err := convertBlock(extendedBlock)

	// Assert
	assert.Nil(t, blockMessage)
	assert.Error(t, err)
}

func TestConvertBlock(t *testing.T) {
	// Arrange
	b := new(block.Builder).
		Build()

	sig, err := crypto.Sign(b.Header().SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	b = b.WithSignature(sig)
	extendedBlock := &chain.ExtendedBlock{Block: b, Obsolete: false}

	// Act
	blockMessage, err := convertBlock(extendedBlock)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, b.Header().Number(), blockMessage.Number)
	assert.Equal(t, b.Header().ParentID(), blockMessage.ParentID)
	assert.Equal(t, uint32(b.Size()), blockMessage.Size)
	assert.Equal(t, b.Header().ParentID(), blockMessage.ParentID)
	assert.Equal(t, b.Header().Timestamp(), blockMessage.Timestamp)
	assert.Equal(t, b.Header().GasLimit(), blockMessage.GasLimit)
	assert.Equal(t, b.Header().Beneficiary(), blockMessage.Beneficiary)
	assert.Equal(t, b.Header().GasUsed(), blockMessage.GasUsed)
	assert.Equal(t, b.Header().TotalScore(), blockMessage.TotalScore)
	assert.Equal(t, b.Header().TxsRoot(), blockMessage.TxsRoot)
	assert.Equal(t, uint32(b.Header().TxsFeatures()), blockMessage.TxsFeatures)
	assert.Equal(t, b.Header().StateRoot(), blockMessage.StateRoot)
	assert.Equal(t, b.Header().ReceiptsRoot(), blockMessage.ReceiptsRoot)
	assert.Equal(t, b.Header().COM(), blockMessage.COM)
	blockSigner, err := b.Header().Signer()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, blockSigner, blockMessage.Signer)
	assert.Equal(t, len(b.Transactions()), len(blockMessage.Transactions))
	assert.Equal(t, false, blockMessage.Obsolete)
}

func TestConvertTransfer(t *testing.T) {
	// Arrange
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)

	// New tx
	transaction, err := new(tx.Builder).
		ChainTag(repo.ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		BlockRef(tx.NewBlockRef(0)).
		BuildAndSign(genesis.DevAccounts()[0].PrivateKey)
	require.NoError(t, err)

	// New block
	blk := new(block.Builder).
		Transaction(transaction).
		Build()

	transfer := &tx.Transfer{
		Sender:    thor.BytesToAddress([]byte("sender")),
		Recipient: thor.BytesToAddress([]byte("recipient")),
		Amount:    big.NewInt(50),
	}

	// Act
	transferMessage, err := convertTransfer(blk.Header(), transaction, 0, transfer, false)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, transfer.Sender, transferMessage.Sender)
	assert.Equal(t, transfer.Recipient, transferMessage.Recipient)
	amount := (*math.HexOrDecimal256)(transfer.Amount)
	assert.Equal(t, amount, transferMessage.Amount)
	assert.Equal(t, blk.Header().ID(), transferMessage.Meta.BlockID)
	assert.Equal(t, blk.Header().Number(), transferMessage.Meta.BlockNumber)
	assert.Equal(t, blk.Header().Timestamp(), transferMessage.Meta.BlockTimestamp)
	assert.Equal(t, transaction.ID(), transferMessage.Meta.TxID)
	origin, err := transaction.Origin()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, origin, transferMessage.Meta.TxOrigin)
	assert.Equal(t, uint32(0), transferMessage.Meta.ClauseIndex)
	assert.Equal(t, false, transferMessage.Obsolete)
}

func TestConvertEventWithBadSignature(t *testing.T) {
	// Arrange
	badSig := bytes.Repeat([]byte{0xf}, 65)

	// New tx
	transaction := new(tx.Builder).
		ChainTag(1).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		BlockRef(tx.NewBlockRef(0)).
		Build().
		WithSignature(badSig[:])

	// New block
	blk := new(block.Builder).
		Transaction(transaction).
		Build()

	// New event
	event := &tx.Event{}

	// Act
	eventMessage, err := convertEvent(blk.Header(), transaction, 0, event, false)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, eventMessage)
}

func TestConvertEvent(t *testing.T) {
	// Arrange
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)

	// New tx
	transaction, err := new(tx.Builder).
		ChainTag(repo.ChainTag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		BlockRef(tx.NewBlockRef(0)).
		BuildAndSign(genesis.DevAccounts()[0].PrivateKey)
	require.NoError(t, err)

	// New block
	blk := new(block.Builder).
		Transaction(transaction).
		Build()

	// New event
	event := &tx.Event{
		Address: thor.BytesToAddress([]byte("address")),
		Topics: []thor.Bytes32{
			{0x01},
			{0x02},
			{0x03},
			{0x04},
			{0x05},
		},
		Data: []byte("data"),
	}

	// Act
	eventMessage, err := convertEvent(blk.Header(), transaction, 0, event, false)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, event.Address, eventMessage.Address)
	assert.Equal(t, hexutil.Encode(event.Data), eventMessage.Data)
	assert.Equal(t, blk.Header().ID(), eventMessage.Meta.BlockID)
	assert.Equal(t, blk.Header().Number(), eventMessage.Meta.BlockNumber)
	assert.Equal(t, blk.Header().Timestamp(), eventMessage.Meta.BlockTimestamp)
	assert.Equal(t, transaction.ID(), eventMessage.Meta.TxID)
	signer, err := transaction.Origin()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, signer, eventMessage.Meta.TxOrigin)
	assert.Equal(t, uint32(0), eventMessage.Meta.ClauseIndex)
	assert.Equal(t, event.Topics, eventMessage.Topics)
	assert.Equal(t, false, eventMessage.Obsolete)
}

func TestEventFilter_Match(t *testing.T) {
	// Create an event filter
	addr := thor.BytesToAddress([]byte("address"))
	filter := &EventFilter{
		Address: &addr,
		Topic0:  &thor.Bytes32{0x01},
		Topic1:  &thor.Bytes32{0x02},
		Topic2:  &thor.Bytes32{0x03},
		Topic3:  &thor.Bytes32{0x04},
		Topic4:  &thor.Bytes32{0x05},
	}

	// Create an event that matches the filter
	event := &tx.Event{
		Address: addr,
		Topics: []thor.Bytes32{
			{0x01},
			{0x02},
			{0x03},
			{0x04},
			{0x05},
		},
	}
	assert.True(t, filter.Match(event))

	// Create an event that does not match the filter address
	event = &tx.Event{
		Address: thor.BytesToAddress([]byte("other_address")),
		Topics: []thor.Bytes32{
			{0x01},
			{0x02},
			{0x03},
			{0x04},
			{0x05},
		},
	}
	assert.False(t, filter.Match(event))

	// Create an event that does not match a filter topic
	event = &tx.Event{
		Address: addr,
		Topics: []thor.Bytes32{
			{0x05},
			{0x04},
			{0x03},
			{0x02},
			{0x01},
		},
	}
	assert.False(t, filter.Match(event))

	// Create an event that does not match a filter topic len
	event = &tx.Event{
		Address: addr,
		Topics:  []thor.Bytes32{{0x01}},
	}
	assert.False(t, filter.Match(event))
}

func TestTransferFilter_Match(t *testing.T) {
	// Create a transfer filter
	origin := thor.BytesToAddress([]byte("origin"))
	sender := thor.BytesToAddress([]byte("sender"))
	recipient := thor.BytesToAddress([]byte("recipient"))
	filter := &TransferFilter{
		TxOrigin:  &origin,
		Sender:    &sender,
		Recipient: &recipient,
	}

	// Create a transfer that matches the filter
	transfer := &tx.Transfer{
		Sender:    thor.BytesToAddress([]byte("sender")),
		Recipient: thor.BytesToAddress([]byte("recipient")),
		Amount:    big.NewInt(100),
	}
	assert.True(t, filter.Match(transfer, origin))

	// Create a transfer that does not match the filter
	transfer = &tx.Transfer{
		Sender:    thor.BytesToAddress([]byte("other_sender")),
		Recipient: thor.BytesToAddress([]byte("recipient")),
		Amount:    big.NewInt(100),
	}
	assert.False(t, filter.Match(transfer, origin))
	assert.False(t, filter.Match(transfer, thor.BytesToAddress(nil)))
	transfer = &tx.Transfer{
		Sender:    sender,
		Recipient: thor.BytesToAddress([]byte("other_recipient")),
		Amount:    big.NewInt(100),
	}
	assert.False(t, filter.Match(transfer, origin))
}
