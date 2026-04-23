// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

// Named block tags per the Ethereum JSON-RPC spec. "pending" collapses to
// "latest" for most read paths today (Thor exposes pool visibility through
// pool.Dump rather than a synthetic pending block).
const (
	TagLatest    = "latest"
	TagEarliest  = "earliest"
	TagPending   = "pending"
	TagSafe      = "safe"
	TagFinalized = "finalized"
)

// BlockTag is the EIP-1898 tag-or-hash-or-number parameter. Exactly one of
// the four state fields is populated after UnmarshalJSON returns without
// error; Resolve normalizes that into a *chain.BlockSummary.
type BlockTag struct {
	tagName          string
	blockNumber      *uint64
	blockHash        *thor.Bytes32
	requireCanonical bool
}

// IsPending reports whether the tag is explicitly "pending" — handlers that
// surface pool visibility (eth_sendRawTransaction, eth_getTransactionCount)
// use this to branch, while plain readers treat it as "latest".
func (b *BlockTag) IsPending() bool { return b.tagName == TagPending }

// TagName returns the tag string if the tag was a named tag (latest /
// earliest / pending / safe / finalized), else "".
func (b *BlockTag) TagName() string { return b.tagName }

// UnmarshalJSON accepts the 5 EIP-1898 shapes:
//
//  1. "latest" | "earliest" | "pending" | "safe" | "finalized"
//  2. "0x<hex>"  (block number quantity)
//  3. "0x<64-hex>" (bare block hash — DATA)
//  4. { "blockNumber": "0x<hex>" }
//  5. { "blockHash": "0x<64-hex>", "requireCanonical"?: bool }
//
// Ambiguity between shape 2 and shape 3 is resolved by length: a 66-char
// ("0x" + 64 hex) string is treated as a hash, everything else as a quantity.
func (b *BlockTag) UnmarshalJSON(data []byte) error {
	*b = BlockTag{}

	// Try string form first (covers shapes 1-3).
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return b.unmarshalString(s)
	}

	// Object form (shapes 4-5).
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err == nil {
		return b.unmarshalObject(obj)
	}

	return fmt.Errorf("block tag must be a string or object, got %s", truncate(data))
}

func (b *BlockTag) unmarshalString(s string) error {
	switch s {
	case TagLatest, TagEarliest, TagPending, TagSafe, TagFinalized:
		b.tagName = s
		return nil
	}

	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return fmt.Errorf("block tag %q must be a named tag or 0x-prefixed hex", s)
	}

	// 0x + 64 hex chars → hash. Anything shorter → quantity.
	if len(s) == 66 {
		var h thor.Bytes32
		if err := h.UnmarshalJSON([]byte(strconv.Quote(s))); err != nil {
			return fmt.Errorf("block hash: %w", err)
		}
		b.blockHash = &h
		return nil
	}

	num, err := parseHexUint64(s)
	if err != nil {
		return fmt.Errorf("block number: %w", err)
	}
	b.blockNumber = &num
	return nil
}

func (b *BlockTag) unmarshalObject(obj map[string]json.RawMessage) error {
	_, hasNum := obj["blockNumber"]
	hashRaw, hasHash := obj["blockHash"]

	if hasNum && hasHash {
		return errors.New("block tag: blockNumber and blockHash are mutually exclusive")
	}
	if !hasNum && !hasHash {
		return errors.New("block tag: object must contain blockNumber or blockHash")
	}

	if hasNum {
		var qty string
		if err := json.Unmarshal(obj["blockNumber"], &qty); err != nil {
			return fmt.Errorf("block tag.blockNumber: %w", err)
		}
		num, err := parseHexUint64(qty)
		if err != nil {
			return fmt.Errorf("block tag.blockNumber: %w", err)
		}
		b.blockNumber = &num
		return nil
	}

	// blockHash path — optionally with requireCanonical.
	var h thor.Bytes32
	if err := json.Unmarshal(hashRaw, &h); err != nil {
		return fmt.Errorf("block tag.blockHash: %w", err)
	}
	b.blockHash = &h

	if rc, ok := obj["requireCanonical"]; ok {
		if err := json.Unmarshal(rc, &b.requireCanonical); err != nil {
			return fmt.Errorf("block tag.requireCanonical: %w", err)
		}
	}
	return nil
}

// Resolve walks the repo / bft engine to turn the tag into a concrete block
// summary plus the canonical chain view rooted at the best block. The chain
// view is returned so callers can do downstream lookups
// (GetTransaction / GetBlockHeader by number) without repeating the work.
//
// For a blockHash tag, when requireCanonical is true and the block isn't on
// the canonical chain, Resolve returns an RPC error with
// reason=block_not_canonical. Callers translate the Go error shape to a
// *RPCError via ToRPCError (below).
//
// Unknown hashes, numbers past head, and a "finalized" request before
// Justified() has ever returned all surface as regular errors; only the
// canonical-check path uses the reason-coded error.
func (b *BlockTag) Resolve(repo *chain.Repository, bftEngine bft.Committer) (*chain.Chain, *chain.BlockSummary, error) {
	bestChain := repo.NewBestChain()

	switch {
	case b.blockHash != nil:
		summary, err := repo.GetBlockSummary(*b.blockHash)
		if err != nil {
			return nil, nil, fmt.Errorf("block hash not found: %s", b.blockHash.String())
		}
		if b.requireCanonical {
			canonicalID, cerr := bestChain.GetBlockID(summary.Header.Number())
			if cerr != nil || canonicalID != *b.blockHash {
				return nil, nil, errBlockNotCanonical{hash: *b.blockHash}
			}
		}
		return bestChain, summary, nil

	case b.blockNumber != nil:
		if *b.blockNumber > math.MaxUint32 {
			return nil, nil, fmt.Errorf("block number %d exceeds uint32 max", *b.blockNumber)
		}
		summary, err := bestChain.GetBlockSummary(uint32(*b.blockNumber))
		if err != nil {
			return nil, nil, fmt.Errorf("block number %d not found", *b.blockNumber)
		}
		return bestChain, summary, nil

	case b.tagName != "":
		return resolveNamedTag(b.tagName, repo, bftEngine, bestChain)
	}

	// Unmarshal guards should prevent this; defensive fallback mirrors
	// the "latest" choice most JSON-RPC implementations make when the
	// caller passed {} / no tag at all.
	return bestChain, repo.BestBlockSummary(), nil
}

func resolveNamedTag(name string, repo *chain.Repository, bftEngine bft.Committer, bestChain *chain.Chain) (*chain.Chain, *chain.BlockSummary, error) {
	switch name {
	case TagLatest, TagPending:
		return bestChain, repo.BestBlockSummary(), nil
	case TagEarliest:
		summary, err := bestChain.GetBlockSummary(0)
		if err != nil {
			return nil, nil, fmt.Errorf("earliest block not available: %w", err)
		}
		return bestChain, summary, nil
	case TagFinalized:
		id := bftEngine.Finalized()
		if id.IsZero() {
			return nil, nil, errors.New("finalized block not available yet")
		}
		summary, err := repo.GetBlockSummary(id)
		if err != nil {
			return nil, nil, fmt.Errorf("finalized block lookup: %w", err)
		}
		return bestChain, summary, nil
	case TagSafe:
		id, err := bftEngine.Justified()
		if err != nil {
			return nil, nil, fmt.Errorf("safe block lookup: %w", err)
		}
		if id.IsZero() {
			// Fallback per spec §8.3: safe is justified, but before any
			// checkpoint is justified fall back to finalized.
			id = bftEngine.Finalized()
			if id.IsZero() {
				return nil, nil, errors.New("safe block not available yet")
			}
		}
		summary, err := repo.GetBlockSummary(id)
		if err != nil {
			return nil, nil, fmt.Errorf("safe block lookup: %w", err)
		}
		return bestChain, summary, nil
	}
	return nil, nil, fmt.Errorf("unknown block tag %q", name)
}

// errBlockNotCanonical is the typed error returned when
// requireCanonical=true and the supplied blockHash is reorg'd off the best
// chain. ToRPCError maps it to the canonical JSON-RPC reason.
type errBlockNotCanonical struct {
	hash thor.Bytes32
}

func (e errBlockNotCanonical) Error() string {
	return "block " + e.hash.String() + " is not on the canonical chain"
}

// ToRPCError converts a Resolve error into a *RPCError suitable for handler
// returns. Callers that have a more specific mapping should handle that
// error themselves before falling back to ToRPCError.
func ToRPCError(err error) *RPCError {
	if err == nil {
		return nil
	}
	var bnc errBlockNotCanonical
	if errors.As(err, &bnc) {
		return ReasonError(ReasonBlockNotCanonical, err.Error())
	}
	return InvalidParams(err.Error())
}

// --- helpers -------------------------------------------------------------

// parseHexUint64 parses a non-empty "0x"-prefixed hex string as a uint64.
// Bare "0x" is rejected (Ethereum convention is "0x0" for zero). Leading
// zeros in "0x0a" are tolerated to match common client output.
func parseHexUint64(s string) (uint64, error) {
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return 0, fmt.Errorf("hex quantity must be 0x-prefixed: %q", s)
	}
	body := s[2:]
	if body == "" {
		return 0, fmt.Errorf("hex quantity is empty")
	}
	n, err := strconv.ParseUint(body, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid hex quantity %q: %w", s, err)
	}
	return n, nil
}

// truncate shortens a byte slice for error messages.
func truncate(data []byte) string {
	const max = 60
	if len(data) <= max {
		return string(data)
	}
	return string(data[:max]) + "..."
}
