// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rewards

import (
	"fmt"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

var issuedKey = thor.Blake2b([]byte("issued"))

type Rewards struct {
	repo       *chain.Repository
	bft        bft.Committer
	stater     *state.Stater
	forkConfig *thor.ForkConfig
}

func New(repo *chain.Repository, bft bft.Committer, stater *state.Stater, forkConfig *thor.ForkConfig) *Rewards {
	return &Rewards{
		repo,
		bft,
		stater,
		forkConfig,
	}
}

func (r *Rewards) handleGetBlockRewards(w http.ResponseWriter, req *http.Request) error {
	revision, err := restutil.ParseRevision(mux.Vars(req)["revision"], false)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "revision"))
	}

	summary, st, err := restutil.GetSummaryAndState(revision, r.repo, r.bft, r.stater, r.forkConfig)
	if err != nil {
		if r.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}

	hayabusaTime, err := builtin.Energy.Native(st, summary.Header.Timestamp()).GetEnergyGrowthStopTime()
	if err != nil {
		if r.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "hayabusa not active"))
		}
		return err
	}

	if hayabusaTime > summary.Header.Timestamp() {
		return restutil.BadRequest(fmt.Errorf("pre hayabusa block"))
	}

	signer, err := summary.Header.Signer()
	if err != nil {
		if r.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "signer"))
		}
		return err
	}

	staker := builtin.Staker.Native(st)
	_, validationID, err := staker.LookupNode(signer)
	if err != nil {
		if r.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "validator"))
		}
		return err
	}

	revisionBeforeBlock, err := restutil.ParseRevision(summary.Header.ParentID().String(), false)
	if err != nil {
		if r.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "revision"))
		}
		return err
	}

	_, stBeforeBlock, err := restutil.GetSummaryAndState(revisionBeforeBlock, r.repo, r.bft, r.stater, r.forkConfig)
	if err != nil {
		if r.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "block summary"))
		}
		return err
	}

	issuedAtBlock := new(big.Int)
	err = st.DecodeStorage(builtin.Energy.Address, issuedKey, func(raw []byte) error {
		if len(raw) == 0 {
			issuedAtBlock = big.NewInt(0)
			return nil
		}
		return rlp.DecodeBytes(raw, &issuedAtBlock)
	})
	if err != nil {
		if r.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "reward"))
		}
		return err
	}

	issuedBeforeBlock := new(big.Int)
	err = stBeforeBlock.DecodeStorage(builtin.Energy.Address, issuedKey, func(raw []byte) error {
		if len(raw) == 0 {
			issuedBeforeBlock = big.NewInt(0)
			return nil
		}
		return rlp.DecodeBytes(raw, &issuedBeforeBlock)
	})
	if err != nil {
		if r.repo.IsNotFound(err) {
			return restutil.BadRequest(errors.WithMessage(err, "reward"))
		}
		return err
	}

	reward := big.NewInt(0).Sub(issuedAtBlock, issuedBeforeBlock)
	hexOrDecimalReward := math.HexOrDecimal256(*reward)
	return restutil.WriteJSON(w, &api.JSONBlockReward{
		Reward:      &hexOrDecimalReward,
		Master:      &signer,
		ValidatorID: &validationID,
	})
}

func (r *Rewards) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/{revision}").
		Methods(http.MethodGet).
		Name("GET /blocks/reward/{revision}").
		HandlerFunc(restutil.WrapHandlerFunc(r.handleGetBlockRewards))
}
