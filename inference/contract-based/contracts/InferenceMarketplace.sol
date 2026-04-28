// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/utils/cryptography/MerkleProof.sol";

/**
 * InferenceMarketplace — Version 1 (no blob storage)
 *
 * Coordination contract for verifiable LLM inference on VeChain.
 * Inference nodes stake VET, run the model, and post receipts.
 * Checker nodes stake VET, run Freivalds off-chain, and post verdicts.
 * Anyone can submit a bisection fraud proof to slash a cheating prover.
 *
 * Receipt encoding:
 *   x[] / z[] — binary float32 (little-endian) emitted as event data
 *   W_root    — MerkleRoot(rows of hook-layer weight matrix W)
 *   x_root    — MerkleRoot(scaled elements of activation vector x)
 *   z_root    — MerkleRoot(scaled elements of activation vector z)
 *
 * Merkle leaf conventions (OZ-compatible, double-keccak256):
 *   W row i :  keccak256(keccak256(abi.encodePacked(uint256(i), rowBytes)))
 *   x[i]/z[i]: keccak256(keccak256(abi.encodePacked(uint256(i), int256(scaled))))
 *   scale = 1_000_000 (values multiplied before rounding to int256)
 */
contract InferenceMarketplace {

    // ── Constants ────────────────────────────────────────────────────────── //

    uint256 public constant CHALLENGE_WINDOW    = 30;         // blocks
    uint256 public constant MIN_INFERENCE_STAKE = 10 ether;   // 10 VET
    uint256 public constant MIN_CHECKER_STAKE   = 5 ether;    // 5 VET
    uint256 public constant FRAUD_THRESHOLD     = 2;          // fraud votes to slash
    uint256 public constant CHALLENGER_BOND     = 1 ether;    // bond for fraud proof

    // ── Model Registry ───────────────────────────────────────────────────── //

    struct ModelInfo {
        bytes32 wRoot;       // MerkleRoot of hook-layer weight matrix rows
        bool    registered;
    }
    mapping(string => ModelInfo) public models;

    // ── Node Registry ────────────────────────────────────────────────────── //

    struct NodeInfo {
        uint256 stake;
        bool    active;
    }
    mapping(address => NodeInfo) public inferenceNodes;
    mapping(address => NodeInfo) public checkerNodes;

    // ── Request / Response State ─────────────────────────────────────────── //

    struct Request {
        address requester;
        string  model;
        bytes32 inputHash;
        uint32  maxNewTokens;
        uint256 payment;
        uint256 blockNumber;
        bool    responded;
        bool    finalized;
    }
    mapping(bytes32 => Request) public requests;

    struct Response {
        address prover;
        bytes32 outputHash;
        bytes32 modelCommitment; // must equal models[model].wRoot
        bytes32 xRoot;
        bytes32 zRoot;
        uint256 blockNumber;
        bool    slashed;
        uint256 fraudVoteCount;
        uint256 validVoteCount;
        mapping(address => bool) hasVoted;
    }
    mapping(bytes32 => Response) public responses;

    // ── Events ────────────────────────────────────────────────────────────── //

    event ModelRegistered(string indexed model, bytes32 wRoot);

    event InferenceNodeRegistered(address indexed node, uint256 stake);
    event CheckerNodeRegistered(address indexed node, uint256 stake);

    event InferenceRequested(
        bytes32 indexed requestId,
        address indexed requester,
        string          model,
        bytes32         inputHash,
        bytes32         userPubkey,   // for encrypted output delivery (off-chain)
        uint32          maxNewTokens,
        uint256         payment
    );

    // x/z carried as binary float32 blobs in the event — not stored in state.
    // This is the V1 approach: data lives in event logs, readable by all nodes.
    // V2 will replace xEncoded/zEncoded with KZG commitments + blob transactions.
    event InferenceResponded(
        bytes32 indexed requestId,
        address indexed prover,
        bytes32         outputHash,
        bytes32         modelCommitment,
        bytes32         xRoot,
        bytes32         zRoot,
        bytes           xEncoded,
        bytes           zEncoded
    );

    event VerdictSubmitted(
        bytes32 indexed requestId,
        address indexed checker,
        bool            valid,
        string          reason
    );

    event NodeSlashed(address indexed node, uint256 amount, string reason);

    event ResponseFinalized(
        bytes32 indexed requestId,
        address indexed prover,
        uint256         payment
    );

    event FraudProofAccepted(
        bytes32 indexed requestId,
        address indexed challenger,
        uint256         disputedIndex
    );

    // ── Model Registration ────────────────────────────────────────────────── //

    function registerModel(string calldata model, bytes32 wRoot) external {
        models[model] = ModelInfo({ wRoot: wRoot, registered: true });
        emit ModelRegistered(model, wRoot);
    }

    // ── Node Registration ─────────────────────────────────────────────────── //

    function registerInferenceNode() external payable {
        require(msg.value >= MIN_INFERENCE_STAKE, "insufficient stake");
        require(!inferenceNodes[msg.sender].active, "already registered");
        inferenceNodes[msg.sender] = NodeInfo({ stake: msg.value, active: true });
        emit InferenceNodeRegistered(msg.sender, msg.value);
    }

    function registerCheckerNode() external payable {
        require(msg.value >= MIN_CHECKER_STAKE, "insufficient stake");
        require(!checkerNodes[msg.sender].active, "already registered");
        checkerNodes[msg.sender] = NodeInfo({ stake: msg.value, active: true });
        emit CheckerNodeRegistered(msg.sender, msg.value);
    }

    // ── Request Lifecycle ─────────────────────────────────────────────────── //

    function submitRequest(
        string  calldata model,
        bytes32          inputHash,
        bytes32          userPubkey,  // for encrypted output delivery (off-chain)
        uint32           maxNewTokens
    ) external payable returns (bytes32 requestId) {
        require(models[model].registered, "model not registered");
        requestId = keccak256(abi.encodePacked(
            msg.sender, model, inputHash, block.number, block.timestamp
        ));
        require(requests[requestId].blockNumber == 0, "request collision");
        requests[requestId] = Request({
            requester:    msg.sender,
            model:        model,
            inputHash:    inputHash,
            maxNewTokens: maxNewTokens,
            payment:      msg.value,
            blockNumber:  block.number,
            responded:    false,
            finalized:    false
        });
        emit InferenceRequested(requestId, msg.sender, model, inputHash, userPubkey, maxNewTokens, msg.value);
    }

    function submitResponse(
        bytes32          requestId,
        bytes32          outputHash,
        bytes32          modelCommitment,
        bytes32          xRoot,
        bytes32          zRoot,
        bytes   calldata xEncoded,
        bytes   calldata zEncoded
    ) external {
        Request storage req = requests[requestId];
        require(req.blockNumber != 0,              "unknown request");
        require(!req.responded,                    "already responded");
        require(inferenceNodes[msg.sender].active, "not a registered inference node");
        // First fraud gate: model commitment must match the registered W_root.
        // A wrong-model prover cannot pass this check.
        require(
            modelCommitment == models[req.model].wRoot,
            "model commitment mismatch"
        );

        req.responded = true;

        Response storage resp = responses[requestId];
        resp.prover          = msg.sender;
        resp.outputHash      = outputHash;
        resp.modelCommitment = modelCommitment;
        resp.xRoot           = xRoot;
        resp.zRoot           = zRoot;
        resp.blockNumber     = block.number;

        emit InferenceResponded(
            requestId, msg.sender, outputHash, modelCommitment,
            xRoot, zRoot, xEncoded, zEncoded
        );
    }

    // ── Verification ──────────────────────────────────────────────────────── //

    function submitVerdict(
        bytes32        requestId,
        bool           valid,
        string calldata reason
    ) external {
        require(checkerNodes[msg.sender].active, "not a registered checker node");
        Response storage resp = responses[requestId];
        require(resp.prover != address(0),                              "no response yet");
        require(!resp.hasVoted[msg.sender],                             "already voted");
        require(block.number <= resp.blockNumber + CHALLENGE_WINDOW,    "challenge window closed");
        require(!resp.slashed,                                          "already slashed");

        resp.hasVoted[msg.sender] = true;

        if (valid) {
            resp.validVoteCount++;
        } else {
            resp.fraudVoteCount++;
            if (resp.fraudVoteCount >= FRAUD_THRESHOLD) {
                _slash(requestId, "checker consensus FRAUD");
            }
        }

        emit VerdictSubmitted(requestId, msg.sender, valid, reason);
    }

    // ── Bisection Fraud Proof ─────────────────────────────────────────────── //
    //
    // Anyone can submit a fraud proof identifying the disputed index i where
    // z[i] ≠ (W · x)[i].  The contract verifies three Merkle proofs to confirm
    // the claimed values are consistent with the posted commitments, then
    // (in production) computes the dot product on-chain.
    //
    // PoC STUB: Steps 1–3 (Merkle verification) are fully implemented.
    //           Step 4 (on-chain dot product) is stubbed with a comment.
    //           The contract trusts the challenger if all three proofs pass.
    //
    //           A production implementation would:
    //             4a. Decode wRowBytes from float32 little-endian to fixed-point int256[]
    //             4b. Read x_vector from the xEncoded event data (all n elements)
    //             4c. Compute y_i = Σ wRow[j] * x[j]  (one row dot product)
    //             4d. require(y_i != z_i, "no fraud: values match")

    function submitFraudProof(
        bytes32          requestId,
        uint256          disputedIndex,
        bytes   calldata wRowBytes,       // float32 little-endian binary for row i
        bytes32[] calldata wMerklePath,   // W[i] ∈ W_root  (registered model)
        bytes32[] calldata xMerklePath,   // x[i] ∈ x_root  (posted in response)
        bytes32[] calldata zMerklePath,   // z[i] ∈ z_root  (posted in response)
        int256           x_i,             // x[i] scaled by 1_000_000
        int256           z_i              // z[i] scaled by 1_000_000
    ) external payable {
        require(msg.value >= CHALLENGER_BOND, "challenger bond required");

        Request  storage req  = requests[requestId];
        Response storage resp = responses[requestId];
        require(resp.prover != address(0),                           "no response");
        require(!resp.slashed,                                       "already slashed");
        require(block.number <= resp.blockNumber + CHALLENGE_WINDOW, "window closed");

        // Step 1: Verify W[disputedIndex] ∈ registered W_root
        require(
            MerkleProof.verify(wMerklePath, models[req.model].wRoot, _rowLeaf(disputedIndex, wRowBytes)),
            "W Merkle proof invalid"
        );

        // Step 2: Verify x[disputedIndex] ∈ x_root posted in response
        require(
            MerkleProof.verify(xMerklePath, resp.xRoot, _elementLeaf(disputedIndex, x_i)),
            "x Merkle proof invalid"
        );

        // Step 3: Verify z[disputedIndex] ∈ z_root posted in response
        require(
            MerkleProof.verify(zMerklePath, resp.zRoot, _elementLeaf(disputedIndex, z_i)),
            "z Merkle proof invalid"
        );

        // Step 4: [PoC STUB] On-chain dot product verification.
        // In production: decode wRowBytes to fixed-point int256[], read x_vector
        // from the InferenceResponded event, compute y_i = dot(wRow, x_vector),
        // and require(y_i != z_i).  For this PoC we trust the challenger when
        // all three Merkle proofs are valid — the protocol structure is complete.

        // Slash and refund bond
        _slash(requestId, "bisection fraud proof accepted");
        payable(msg.sender).transfer(msg.value);
        emit FraudProofAccepted(requestId, msg.sender, disputedIndex);
    }

    // ── Settlement ────────────────────────────────────────────────────────── //

    function finalizeResponse(bytes32 requestId) external {
        Request  storage req  = requests[requestId];
        Response storage resp = responses[requestId];
        require(req.responded,  "no response submitted");
        require(!req.finalized, "already finalized");
        require(!resp.slashed,  "response was slashed");
        require(
            block.number > resp.blockNumber + CHALLENGE_WINDOW,
            "challenge window still open"
        );

        req.finalized = true;
        uint256 payment = req.payment;
        address prover  = resp.prover;

        if (payment > 0) {
            payable(prover).transfer(payment);
        }
        emit ResponseFinalized(requestId, prover, payment);
    }

    // ── View helpers ──────────────────────────────────────────────────────── //

    function getRequest(bytes32 requestId) external view returns (
        address requester, string memory model, bytes32 inputHash,
        uint32 maxNewTokens, uint256 payment, uint256 blockNum,
        bool responded, bool finalized
    ) {
        Request storage r = requests[requestId];
        return (r.requester, r.model, r.inputHash, r.maxNewTokens,
                r.payment, r.blockNumber, r.responded, r.finalized);
    }

    function getResponse(bytes32 requestId) external view returns (
        address prover, bytes32 outputHash, bytes32 modelCommitment,
        bytes32 xRoot, bytes32 zRoot, uint256 blockNum,
        bool slashed, uint256 fraudVotes, uint256 validVotes
    ) {
        Response storage r = responses[requestId];
        return (r.prover, r.outputHash, r.modelCommitment, r.xRoot, r.zRoot,
                r.blockNumber, r.slashed, r.fraudVoteCount, r.validVoteCount);
    }

    // ── Internal ──────────────────────────────────────────────────────────── //

    function _slash(bytes32 requestId, string memory reason) internal {
        Response storage resp = responses[requestId];
        if (resp.slashed) return;
        resp.slashed = true;

        address prover = resp.prover;
        uint256 stake  = inferenceNodes[prover].stake;
        inferenceNodes[prover].stake  = 0;
        inferenceNodes[prover].active = false;

        emit NodeSlashed(prover, stake, reason);

        // Refund the user's payment
        Request storage req = requests[requestId];
        if (req.payment > 0) {
            payable(req.requester).transfer(req.payment);
        }
        // Slashed stake remains in contract (production: distribute to checkers/challenger)
    }

    // OZ-compatible double-keccak256 leaf for a W matrix row (raw bytes)
    function _rowLeaf(uint256 index, bytes calldata rowBytes) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked(keccak256(abi.encodePacked(index, rowBytes))));
    }

    // OZ-compatible double-keccak256 leaf for a scalar activation element
    function _elementLeaf(uint256 index, int256 value) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked(keccak256(abi.encodePacked(index, value))));
    }
}
