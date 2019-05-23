2018-12-31
based on github.com/ethereum/go-ethereum/core/vm v1.8.10 tag

2019-06-17
based on github.com/ethereum/go-ethereum/core/vm v1.8.27 tag
Due to thor project dependencies on v1.8.14, Some functions are rewritten:

 - crypto.CreateAddress2 is now CreateAddress2 in patch.go file.

Due to structure differences, some features are not supported:
 - readOnly property in interpreter.go
 - WASM interpreter in interpreter.go
 - func gasSStore() in gas_table.go