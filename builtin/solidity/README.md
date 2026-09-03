# Solidity Package

This package provides convenience data structures and types, conceptually similar to Solidity.

Eg. Instead of getting, decoding, encoding and storing, you can initialise a `uint256` and use the `set` and `get`
functions to interact with it.

```go
type Builtin struct {
    addr thor.Address
    state *state.State
    totalStake *solidity.Uint256
}

func NewBuiltIn(addr thor.Address, state *state.State) *Builtin {
    return &Builtin{
        addr: addr,
        state: state,
        totalStake: solidity.NewUint256(addr, state, thor.Bytes32{0x0},
    }
}

func (b *Builtin) SetTotalStake(value *big.Int) {
    b.totalStake.Set(value)
}
```
