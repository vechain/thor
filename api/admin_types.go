// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"time"
)

type LogStatus struct {
	Enabled bool `json:"enabled"`
}

type HealthStatus struct {
	Healthy              bool       `json:"healthy"`
	BestBlockTime        *time.Time `json:"bestBlockTime"`
	PeerCount            int        `json:"peerCount"`
	IsNetworkProgressing bool       `json:"isNetworkProgressing"`
	NodeMaster           *string    `json:"nodeMaster"`
	Beneficiary          *string    `json:"beneficiary"`
}

type LogLevelRequest struct {
	Level string `json:"level"`
}

type LogLevelResponse struct {
	CurrentLevel string `json:"currentLevel"`
}
