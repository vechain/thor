// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thorclient

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDependencyClosure checks transitively what the light-client depguard rule in
// .golangci.yml checks for direct imports only: the client-side packages must not
// depend on node internals. Keep both lists in sync.
func TestDependencyClosure(t *testing.T) {
	const module = "github.com/vechain/thor/v2/"

	// A trailing "/" denies subpackages only, a trailing "$" the package only,
	// anything else the package and its subpackages.
	denied := []string{
		"api/",
		"builtin$",
		"builtin/staker",
		"bft",
		"chain",
		"cmd",
		"comm",
		"logdb",
		"muxdb",
		"p2p",
		"runtime",
		"state",
		"test",
		"txpool",
		"vm",
	}

	out, err := exec.Command(
		"go", "list", "-deps",
		module+"thorclient/...",
		module+"builtin/contracts",
		module+"api",
	).CombinedOutput()
	require.NoError(t, err, string(out))

	var violations []string
	for pkg := range strings.FieldsSeq(string(out)) {
		rel, ok := strings.CutPrefix(pkg, module)
		if !ok {
			continue
		}
		for _, d := range denied {
			var match bool
			switch {
			case strings.HasSuffix(d, "/"):
				match = strings.HasPrefix(rel, d)
			case strings.HasSuffix(d, "$"):
				match = rel == strings.TrimSuffix(d, "$")
			default:
				match = rel == d || strings.HasPrefix(rel, d+"/")
			}
			if match {
				violations = append(violations, pkg)
				break
			}
		}
	}
	require.Empty(t, violations,
		"client-side packages depend on node internals; find the importer with: go list -deps -f '{{.ImportPath}}: {{.Imports}}' ./thorclient/...")
}
