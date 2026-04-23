// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethview

import (
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

// emptyUnclesHash is keccak256(RLP([])) — the Ethereum-mandated constant for
// a block with no uncles. Thor has no uncle concept; we always emit this.
var emptyUnclesHash = thor.MustParseBytes32("0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")

// BlockObject is the eth-shape block view. `transactions` is either a list of
// hashes (fullTx=false) or a list of *TransactionObject (fullTx=true).
type BlockObject struct {
	Number           hexutil.Uint64 `json:"number"`
	Hash             thor.Bytes32   `json:"hash"`
	ParentHash       thor.Bytes32   `json:"parentHash"`
	Sha3Uncles       thor.Bytes32   `json:"sha3Uncles"`
	LogsBloom        hexutil.Bytes  `json:"logsBloom"`
	TransactionsRoot thor.Bytes32   `json:"transactionsRoot"`
	StateRoot        thor.Bytes32   `json:"stateRoot"`
	ReceiptsRoot     thor.Bytes32   `json:"receiptsRoot"`
	Miner            thor.Address   `json:"miner"`
	Difficulty       *hexutil.Big   `json:"difficulty"`
	TotalDifficulty  *hexutil.Big   `json:"totalDifficulty"`
	ExtraData        hexutil.Bytes  `json:"extraData"`
	Size             hexutil.Uint64 `json:"size"`
	GasLimit         hexutil.Uint64 `json:"gasLimit"`
	GasUsed          hexutil.Uint64 `json:"gasUsed"`
	Timestamp        hexutil.Uint64 `json:"timestamp"`
	Transactions     any            `json:"transactions"` // []thor.Bytes32 or []*TransactionObject
	Uncles           []thor.Bytes32 `json:"uncles"`
	MixHash          thor.Bytes32   `json:"mixHash"`
	Nonce            hexutil.Bytes  `json:"nonce"`
	BaseFeePerGas    *hexutil.Big   `json:"baseFeePerGas,omitempty"`
}

// ProjectBlock maps a native *block.Block into a BlockObject. When fullTx is
// true the caller-supplied lookupTxMeta (which returns the per-tx metadata
// needed by ProjectTx) is invoked for each tx; any tx that yields
// ErrNotRepresentable causes the whole projection to fail with
// ErrBlockContainsNonRepresentable.
//
// When fullTx is false the transactions list is rendered as CanonicalTxID
// hashes and this function never fails due to representability.
//
// `miner` is the block proposer (header.Signer()); on decode failure the zero
// address is returned (Thor's block objects are always well-formed by the
// time they reach this layer, so this is a defensive fallback rather than a
// real code path).
func ProjectBlock(
	blk *block.Block,
	fullTx bool,
	lookupTxMeta func(txIdx int) TxMeta,
) (*BlockObject, error) {
	header := blk.Header()
	txs := blk.Transactions()

	miner, _ := header.Signer()

	obj := &BlockObject{
		Number:           hexutil.Uint64(header.Number()),
		Hash:             header.ID(),
		ParentHash:       header.ParentID(),
		Sha3Uncles:       emptyUnclesHash,
		LogsBloom:        zeroLogsBloom,
		TransactionsRoot: header.TxsRoot(),
		StateRoot:        header.StateRoot(),
		ReceiptsRoot:     header.ReceiptsRoot(),
		Miner:            miner,
		Difficulty:       (*hexutil.Big)(nil),
		TotalDifficulty:  (*hexutil.Big)(nil),
		ExtraData:        hexutil.Bytes{},
		Size:             hexutil.Uint64(blk.Size()),
		GasLimit:         hexutil.Uint64(header.GasLimit()),
		GasUsed:          hexutil.Uint64(header.GasUsed()),
		Timestamp:        hexutil.Uint64(header.Timestamp()),
		Uncles:           []thor.Bytes32{},
		MixHash:          thor.Bytes32{},
		Nonce:            make(hexutil.Bytes, 8),
	}

	if baseFee := header.BaseFee(); baseFee != nil {
		obj.BaseFeePerGas = (*hexutil.Big)(baseFee)
	}

	if !fullTx {
		hashes := make([]thor.Bytes32, len(txs))
		for i, t := range txs {
			hashes[i] = t.CanonicalTxID()
		}
		obj.Transactions = hashes
		return obj, nil
	}

	fullList := make([]*TransactionObject, 0, len(txs))
	for i, t := range txs {
		var meta TxMeta
		if lookupTxMeta != nil {
			meta = lookupTxMeta(i)
		}
		txObj, err := ProjectTx(t, meta)
		if err != nil {
			if err == ErrNotRepresentable {
				return nil, ErrBlockContainsNonRepresentable
			}
			return nil, err
		}
		fullList = append(fullList, txObj)
	}
	obj.Transactions = fullList
	return obj, nil
}
