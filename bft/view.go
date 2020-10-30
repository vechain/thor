package bft

import (
	"encoding/binary"
	"errors"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

// MaxByzantineNodes - Maximum number of Byzatine nodes, i.e., f
const MaxByzantineNodes = 33

// QC = N - f
const QC = int(thor.MaxBlockProposers) - MaxByzantineNodes

type view struct {
	branch        *chain.Chain
	first         uint32
	nv            map[thor.Address]uint8
	pp            map[thor.Bytes32]map[thor.Address]uint8
	pc            map[thor.Bytes32]map[thor.Address]uint8
	hasConflictPC bool
}

// newView construct a view object starting with the block referred by `id`
func newView(branch *chain.Chain, first uint32) (v *view, err error) {
	var (
		i      = first //block.Number(first)
		maxNum = block.Number(branch.HeadID())

		blk    *block.Block
		pp, pc thor.Bytes32
	)

	blk, err = branch.GetBlock(i)
	if err != nil {
		return nil, err
	}
	// If block b is the first block of a view, then
	// 		nv = [blkNum | 00 ... 0]
	if !isValidFirstNV(blk) {
		return nil, errors.New("Invalid NV value for the first block of the view")
	}

	firstID := blk.Header().ID()

	v = &view{
		branch: branch,
		first:  first,
		nv:     make(map[thor.Address]uint8),
		pp:     make(map[thor.Bytes32]map[thor.Address]uint8),
		pc:     make(map[thor.Bytes32]map[thor.Address]uint8),

		hasConflictPC: false,
	}

	for {
		pp = blk.Header().PP()
		pc = blk.Header().PC()

		if _, ok := v.pp[pp]; !pp.IsZero() && !ok {
			v.pp[pp] = make(map[thor.Address]uint8)
		}
		if _, ok := v.pc[pc]; !pc.IsZero() && !ok {
			v.pc[pc] = make(map[thor.Address]uint8)
		}

		signers := getSigners(blk)
		for _, signer := range signers {
			v.nv[signer] = v.nv[signer] + 1
			if !pp.IsZero() {
				v.pp[pp][signer] = v.pp[pp][signer] + 1
			}
			if !pc.IsZero() {
				v.pc[pc][signer] = v.pc[pc][signer] + 1
			}
		}

		if !v.hasConflictPC && !pc.IsZero() && !branch.IsOnChain(pc) {
			v.hasConflictPC = true
		}

		i = i + 1
		if i > maxNum {
			break
		}
		blk, err = branch.GetBlock(i)
		if err != nil {
			return nil, err
		}
		if blk.Header().NV() != firstID {
			break
		}
	}

	return
}

func (v *view) getFirstBlockID() (id thor.Bytes32) {
	id, err := v.branch.GetBlockID(v.first)
	if err != nil {
		panic(err)
	}
	return
}

func (v *view) ifHasConflictPC() bool {
	return v.hasConflictPC
}

func (v *view) ifHasQCForNV() bool {
	return len(v.nv) >= QC
}

func (v *view) ifHasQCForPP() (bool, thor.Bytes32) {
	for pp := range v.pp {
		if len(v.pp[pp]) >= QC {
			return true, pp
		}
	}
	return false, thor.Bytes32{}
}

func (v *view) ifHasQCForPC() (bool, thor.Bytes32) {
	for pc := range v.pc {
		if len(v.pc[pc]) >= QC {
			return true, pc
		}
	}
	return false, thor.Bytes32{}
}

// getPCNum gets the number of signatures on the input pc value
func (v *view) getNumSigOnPC(pc thor.Bytes32) int {
	return len(v.pc[pc])
}

func getSigners(blk *block.Block) (endorsors []thor.Address) {
	header := blk.Header()
	proposer, _ := header.Signer()
	msg := block.NewProposal(
		header.ParentID(),
		header.TxsRoot(),
		header.GasLimit(),
		header.Timestamp(),
	).AsMessage(proposer)

	bss := blk.BackerSignatures()
	for _, bs := range bss {
		pub, err := crypto.SigToPub(thor.Blake2b(msg, bs.Proof()).Bytes(), bs.Signature())
		if err != nil {
			panic(err)
		}
		endorsors = append(endorsors, thor.Address(crypto.PubkeyToAddress(*pub)))
	}

	endorsors = append(endorsors, proposer)
	return
}

// If block b is the first block of a view, then
// 		nv = [blkNum | 00 ... 0]
func isValidFirstNV(first *block.Block) bool {
	nv := first.Header().NV()

	if block.Number(nv) != first.Header().Number() {
		return false
	}

	binary.BigEndian.PutUint32(nv[:], uint32(0))

	return nv.IsZero()
}

// GenNVforFirstBlock computes the nv value for the first block of a view
func GenNVforFirstBlock(num uint32) (nv thor.Bytes32) {
	binary.BigEndian.PutUint32(nv[:], num)
	return
}
