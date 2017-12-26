package schedule_test

import (
	"testing"

	"github.com/vechain/thor/consensus/schedule"
)

func TestSchedule(t *testing.T) {
	_ = schedule.New(
		nil,
		nil,
		0,
		0,
	)
}
