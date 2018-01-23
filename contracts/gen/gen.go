package gen

//go:generate solc --optimize --overwrite --bin-runtime --abi -o ./compiled Authority.sol Energy.sol Params.sol Voting.sol
//go:generate go-bindata -nometadata -pkg gen -o bindata.go compiled/
