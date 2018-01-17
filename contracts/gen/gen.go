package gen

//go:generate solc --optimize --overwrite --bin-runtime --abi -o ./compiled ../../_contracts/contracts/Authority.sol ../../_contracts/contracts/Energy.sol ../../_contracts/contracts/Params.sol
//go:generate go-bindata -nometadata -pkg gen -o bindata.go ./compiled/Authority.abi ./compiled/Authority.bin-runtime ./compiled/Energy.abi ./compiled/Energy.bin-runtime ./compiled/Params.abi ./compiled/Params.bin-runtime
