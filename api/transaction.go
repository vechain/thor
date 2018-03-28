package api

import (
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/api/types"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/txpool"
)

//TransactionInterface for manage block with chain
type TransactionInterface struct {
	chain  *chain.Chain
	txPool *txpool.TxPool
}

//NewTransactionInterface return a BlockMananger by chain
func NewTransactionInterface(chain *chain.Chain, txPool *txpool.TxPool) *TransactionInterface {
	return &TransactionInterface{
		chain:  chain,
		txPool: txPool,
	}
}

//GetTransactionByID return transaction by transaction id
func (ti *TransactionInterface) GetTransactionByID(txID thor.Hash) (*types.Transaction, error) {

	if pengdingTransaction := ti.txPool.GetTransaction(txID); pengdingTransaction != nil {
		return types.ConvertTransaction(pengdingTransaction)
	}
	tx, location, err := ti.chain.GetTransaction(txID)
	if err != nil {
		return nil, err
	}

	t, err := types.ConvertTransaction(tx)
	if err != nil {
		return nil, err
	}

	block, err := ti.chain.GetBlock(location.BlockID)
	if err != nil {
		return nil, err
	}

	t.BlockID = location.BlockID
	t.BlockNumber = block.Header().Number()
	t.Index = math.HexOrDecimal64(location.Index)
	return t, nil
}

//GetTransactionReceiptByID get tx's receipt
func (ti *TransactionInterface) GetTransactionReceiptByID(txID thor.Hash) (*types.Receipt, error) {
	rece, err := ti.chain.GetTransactionReceipt(txID)
	if err != nil {
		if ti.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	receipt := types.ConvertReceipt(rece)
	return receipt, nil
}

//SendRawTransaction send a raw transactoion
func (ti *TransactionInterface) SendRawTransaction(raw *types.RawTransaction) (*thor.Hash, error) {
	transaction, err := types.BuildRawTransaction(raw)
	if err != nil {
		return nil, err
	}
	if err := ti.txPool.Add(transaction); err != nil {
		return nil, err
	}
	txID := transaction.ID()
	return &txID, nil
}

//GetContractInputData get contract input with method and args
func (ti *TransactionInterface) GetContractInputData(contractAddr thor.Address, abiData string, methodName string, args ...interface{}) (input []byte, err error) {
	abi, err := abi.New([]byte(abiData))
	if err != nil {
		return nil, err
	}

	method := abi.MethodByName(methodName)
	if method == nil {
		return nil, errors.New("method not found")
	}
	data, err := method.EncodeInput(args...)
	if err != nil {
		return nil, err
	}
	return data, nil
}
