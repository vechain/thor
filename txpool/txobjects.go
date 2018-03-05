package txpool

//TxObjects array of TxObject
type TxObjects []*TxObject

func (objs TxObjects) Len() int {
	return len(objs)
}

func (objs TxObjects) Less(i, j int) bool {
	return objs[i].OverallGP().Cmp(objs[j].OverallGP()) > 0
}

func (objs TxObjects) Swap(i, j int) {
	objs[i], objs[j] = objs[j], objs[i]
}

//Push Push
func (objs *TxObjects) Push(x interface{}) {
	*objs = append(*objs, x.(*TxObject))
}

//Pop Pop
func (objs *TxObjects) Pop() interface{} {
	old := *objs
	n := len(old)
	x := old[n-1]
	*objs = old[0 : n-1]
	return x
}
