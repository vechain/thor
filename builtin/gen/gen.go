package gen

//go:generate rm -rf ./compiled/
//go:generate solc --optimize --overwrite --bin-runtime --abi -o ./compiled All.sol
//go:generate go-bindata -nometadata -pkg gen -o bindata.go compiled/
