package builtin

// Executor binder of `Executor` contract.
var Executor = func() *executor {
	c := loadContract("Executor")
	return &executor{
		c,
	}
}()

type executor struct {
	*contract
}
