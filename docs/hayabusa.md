# Hayabusa

## Staker

- Address: 0x00000000000000000000000000005374616b6572

```bash
make thor
./bin/thor solo
curl http://localhost:8669/accounts/0x00000000000000000000000000005374616b6572/code
```


## Important Packages

### `builtin/staker`

- Core logic of the staker contract
- Contains protocol logic to activate / exit validators
- Contains implementation methods of `staker.sol` in `builtin/gen`

### `builtin/energy`

- Some modifications here to stop energy growth when PoS becomes active and distribute rewards to block proposer/ stargate.

### `consensus`

- On `master` we had `consensus/validator.go`. We now have 1 validator file for each PoA and PoS to validator block proposers.
- `validator.go` checks the fork config and checks if PoS is active and proxies calls the relevant function to verify the block proposer.
- Rewards are distributed in the `verifyBlock` function in `validator.go` right before the state is committed.

### `packer`

- Similar to `consensus`, but now we have a PoA scheduler and PoS scheduler. Follows the same flow as `consensus`.

### `pos`

- Quite similar to the `poa` package. It creates a scheduler. This contains the algorithm to select the next block proposer, leveraging the validator's weights and the VRF block outputs as a source of randomness.

