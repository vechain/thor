package txpool

type transactions []*transaction

func (txs transactions) Len() int {
	return len(txs)
}

func (txs transactions) Less(i, j int) bool {
	//TODO 排序标准
	return txs[i].tx.GasPrice().Cmp(txs[j].tx.GasPrice()) < 0
}

func (txs transactions) Swap(i, j int) {
	txs[i], txs[j] = txs[j], txs[i]
}

func (txs *transactions) Push(x interface{}) {
	*txs = append(*txs, x.(*transaction))
}

func (txs *transactions) Pop() interface{} {
	old := *txs
	n := len(old)
	x := old[n-1]
	*txs = old[0 : n-1]
	return x
}
