package renewal

import "math/big"

type Renewal struct {
	ChangeTVL            *big.Int
	ChangeWeight         *big.Int
	QueuedDecrease       *big.Int
	QueuedDecreaseWeight *big.Int
}

type Exit struct {
	ExitedTVL            *big.Int
	ExitedTVLWeight      *big.Int
	QueuedDecrease       *big.Int
	QueuedDecreaseWeight *big.Int
}
