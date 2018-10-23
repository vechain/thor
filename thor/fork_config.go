package thor

// ForkConfig config for a fork.
type ForkConfig struct {
	BlockNumber uint32
}

// forks
var (
	FixTransferLogFork = ForkConfig{
		BlockNumber: 1150000,
	}
)
