package consensus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/thor"
)

func Test_calcScore(t *testing.T) {
	assert := assert.New(t)

	proposer1 := schedule.Proposer{
		Address: thor.Address{1},
	}
	proposer1.SetAbsent(true)

	proposer2 := schedule.Proposer{
		Address: thor.Address{2},
	}
	proposer2.SetAbsent(false)

	proposer3 := schedule.Proposer{
		Address: thor.Address{3},
	}
	proposer3.SetAbsent(false)

	proposer4 := schedule.Proposer{
		Address: thor.Address{4},
	}
	proposer4.SetAbsent(false)

	proposers := []schedule.Proposer{
		proposer1,
		proposer2,
		proposer3,
		proposer4,
	}

	assert.Equal(calcScore(proposers, proposers), uint64(3))
}
