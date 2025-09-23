package gen

import (
	"embed"
	"encoding/hex"
)

//go:embed compiled
var fs embed.FS

func MustABI(name string) []byte {
	data, err := fs.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return data
}

func MustBIN(name string) []byte {
	data, err := fs.ReadFile(name)
	if err != nil {
		panic(err)
	}
	bytes, err := hex.DecodeString(string(data))
	if err != nil {
		panic(err)
	}
	return bytes
}
