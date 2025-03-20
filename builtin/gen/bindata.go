package gen

import (
	"embed"
	"strings"
)

//go:embed compiled
var fs embed.FS

// MustAsset ensures that the asset is available and returns its contents.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	if !strings.HasSuffix(name, ".abi") && !strings.HasSuffix(name, ".bin-runtime") {
		panic("asset: Asset(" + name + "): not a valid asset name")
	}

	data, err := fs.ReadFile(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return data
}
