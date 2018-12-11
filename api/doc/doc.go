// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package doc

//go:generate go-bindata -nometadata -ignore=.DS_Store -pkg doc -o bindata.go swagger-ui/... thor.yaml

import (
	yaml "gopkg.in/yaml.v2"
)

//Version open api version
func Version() string {
	return version
}

var version string

type openAPIInfo struct {
	Info struct {
		Version string
	}
}

func init() {
	var oai openAPIInfo
	if err := yaml.Unmarshal(MustAsset("thor.yaml"), &oai); err != nil {
		panic(err)
	}
	version = oai.Info.Version
}
