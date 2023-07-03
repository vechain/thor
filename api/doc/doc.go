// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package doc

import (
	"embed"

	"gopkg.in/yaml.v2"
)

//go:embed swagger-ui thor.yaml
var FS embed.FS
var version string

// Version open api version
func Version() string {
	return version
}

type openAPIInfo struct {
	Info struct {
		Version string
	}
}

func init() {
	content, err := FS.ReadFile("thor.yaml")
	if err != nil {
		panic(err)
	}

	var oai openAPIInfo
	if err := yaml.Unmarshal(content, &oai); err != nil {
		panic(err)
	}
	version = oai.Info.Version
}
