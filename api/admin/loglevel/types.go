// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package loglevel

type Request struct {
	Level string `json:"level"`
}

type Response struct {
	CurrentLevel string `json:"currentLevel"`
}
