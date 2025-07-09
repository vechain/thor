// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vechain/thor/v2/comm/proto"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/thor"
)

// VRFManager manages VRF proof collection and distribution
type VRFManager struct {
	communicator  *Communicator
	proofs        map[string]*proto.VRFProof // key: validatorAddress_blockNumber_alphaHash
	proofsMutex   sync.RWMutex
	requests      map[string]chan []*proto.VRFProof // pending requests
	requestsMutex sync.RWMutex
	logger        log.Logger
}

// NewVRFManager creates a new VRF manager
func NewVRFManager(comm *Communicator) *VRFManager {
	return &VRFManager{
		communicator: comm,
		proofs:       make(map[string]*proto.VRFProof),
		requests:     make(map[string]chan []*proto.VRFProof),
		logger:       log.WithContext("pkg", "vrf-manager"),
	}
}

// generateProofKey generates a unique key for a VRF proof
func (vm *VRFManager) generateProofKey(validator thor.Address, blockNumber uint32, alpha []byte) string {
	alphaHash := thor.Blake2b(alpha)
	alphaHashBytes := alphaHash[:]
	return fmt.Sprintf("%s_%d_%x", validator, blockNumber, alphaHashBytes[:8])
}

// BroadcastVRFProof broadcasts a VRF proof to all peers
func (vm *VRFManager) BroadcastVRFProof(validator thor.Address, alpha []byte, proof []byte, blockNumber uint32) {
	vrfProof := &proto.VRFProof{
		ValidatorAddress: validator,
		Alpha:            alpha,
		Proof:            proof,
		BlockNumber:      blockNumber,
		Timestamp:        uint64(time.Now().Unix()),
	}

	// Store locally
	key := vm.generateProofKey(validator, blockNumber, alpha)
	vm.proofsMutex.Lock()
	vm.proofs[key] = vrfProof
	vm.proofsMutex.Unlock()

	// Broadcast to all peers
	peers := vm.communicator.peerSet.Slice()
	for _, peer := range peers {
		vm.communicator.goes.Go(func() {
			if err := proto.NotifyVRFProof(vm.communicator.ctx, peer, vrfProof); err != nil {
				peer.logger.Debug("failed to broadcast VRF proof", "err", err)
			}
		})
	}

	vm.logger.Debug("VRF proof broadcasted", "validator", validator, "block", blockNumber)
}

// CollectVRFProofs collects VRF proofs for a specific block and alpha
func (vm *VRFManager) CollectVRFProofs(ctx context.Context, alpha []byte, blockNumber uint32, validators []thor.Address, timeout time.Duration) (map[thor.Address][]byte, error) {
	validatorProofs := make(map[thor.Address][]byte)

	// First, check what we have locally
	vm.proofsMutex.RLock()
	for _, validator := range validators {
		key := vm.generateProofKey(validator, blockNumber, alpha)
		if proof, exists := vm.proofs[key]; exists {
			validatorProofs[validator] = proof.Proof
		}
	}
	vm.proofsMutex.RUnlock()

	// Find validators we still need proofs from
	missingValidators := make([]thor.Address, 0)
	for _, validator := range validators {
		if _, exists := validatorProofs[validator]; !exists {
			missingValidators = append(missingValidators, validator)
		}
	}

	if len(missingValidators) == 0 {
		return validatorProofs, nil
	}

	// Request missing proofs from peers
	request := &proto.VRFProofRequest{
		Alpha:       alpha,
		BlockNumber: blockNumber,
		Validators:  missingValidators,
	}

	// Create a channel to collect responses
	responseCh := make(chan []*proto.VRFProof, len(vm.communicator.peerSet.Slice()))
	alphaHash := thor.Blake2b(alpha)
	alphaHashBytes := alphaHash[:]
	requestKey := fmt.Sprintf("%x_%d", alphaHashBytes[:8], blockNumber)

	vm.requestsMutex.Lock()
	vm.requests[requestKey] = responseCh
	vm.requestsMutex.Unlock()

	defer func() {
		vm.requestsMutex.Lock()
		delete(vm.requests, requestKey)
		vm.requestsMutex.Unlock()
		close(responseCh)
	}()

	// Send requests to all peers
	peers := vm.communicator.peerSet.Slice()
	for _, peer := range peers {
		vm.communicator.goes.Go(func() {
			proofs, err := proto.RequestVRFProofs(ctx, peer, request)
			if err != nil {
				peer.logger.Debug("failed to request VRF proofs", "err", err)
				return
			}
			select {
			case responseCh <- proofs:
			case <-ctx.Done():
			}
		})
	}

	// Wait for responses with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	collectedProofs := make(map[thor.Address][]byte)
	responseCount := 0

	for {
		select {
		case <-timeoutCtx.Done():
			vm.logger.Debug("VRF proof collection timeout", "block", blockNumber, "collected", len(collectedProofs), "requested", len(missingValidators))
			// Merge with local proofs
			for validator, proof := range collectedProofs {
				validatorProofs[validator] = proof
			}
			return validatorProofs, nil

				case proofs := <-responseCh:
			responseCount++
			for _, proof := range proofs {
				if proof.BlockNumber == blockNumber && len(proof.Alpha) > 0 {
					proofAlphaHash := thor.Blake2b(proof.Alpha)
					requestAlphaHash := thor.Blake2b(alpha)
					if proofAlphaHash == requestAlphaHash {
						collectedProofs[proof.ValidatorAddress] = proof.Proof
						
						// Store for future use
						key := vm.generateProofKey(proof.ValidatorAddress, proof.BlockNumber, proof.Alpha)
						vm.proofsMutex.Lock()
						vm.proofs[key] = proof
						vm.proofsMutex.Unlock()
					}
				}
			}

			// Check if we have all the proofs we need
			if len(collectedProofs) >= len(missingValidators) || responseCount >= len(peers) {
				// Merge with local proofs
				for validator, proof := range collectedProofs {
					validatorProofs[validator] = proof
				}
				return validatorProofs, nil
			}
		}
	}
}

// HandleVRFProof handles incoming VRF proof messages
func (vm *VRFManager) HandleVRFProof(proof *proto.VRFProof) {
	key := vm.generateProofKey(proof.ValidatorAddress, proof.BlockNumber, proof.Alpha)

	vm.proofsMutex.Lock()
	vm.proofs[key] = proof
	vm.proofsMutex.Unlock()

	vm.logger.Debug("VRF proof received", "validator", proof.ValidatorAddress, "block", proof.BlockNumber)
}

// HandleVRFProofRequest handles incoming VRF proof requests
func (vm *VRFManager) HandleVRFProofRequest(request *proto.VRFProofRequest) []*proto.VRFProof {
	var response []*proto.VRFProof

	vm.proofsMutex.RLock()
	for _, validator := range request.Validators {
		key := vm.generateProofKey(validator, request.BlockNumber, request.Alpha)
		if proof, exists := vm.proofs[key]; exists {
			response = append(response, proof)
		}
	}
	vm.proofsMutex.RUnlock()

	vm.logger.Debug("VRF proof request handled", "requested", len(request.Validators), "found", len(response))
	return response
}



// CleanupOldProofs removes proofs older than the given block number
func (vm *VRFManager) CleanupOldProofs(currentBlock uint32) {
	vm.proofsMutex.Lock()
	defer vm.proofsMutex.Unlock()

	// Keep proofs for the last 10 blocks
	cutoffBlock := currentBlock - 10
	if currentBlock < 10 {
		cutoffBlock = 0
	}

	for key, proof := range vm.proofs {
		if proof.BlockNumber < cutoffBlock {
			delete(vm.proofs, key)
		}
	}
}
