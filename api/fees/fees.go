package fees

import "github.com/vechain/thor/v2/chain"

type Fees struct {
	repo *chain.Repository
}

func New(repo *chain.Repository) *Fees {
	return &Fees{
		repo,
	}
}