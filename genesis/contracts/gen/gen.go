package gen

//go:generate solc --overwrite --bin --abi -o ./compiled ../../../_contracts/contracts/Authority.sol ../../../_contracts/contracts/Energy.sol
//go:generate go-bindata -nometadata -pkg gen -o bindata.go ./compiled/Authority.abi ./compiled/Authority.bin ./compiled/Energy.abi ./compiled/Energy.bin
