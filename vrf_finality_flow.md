# VRF Finality Flow

```mermaid
graph TD
    A[Start] --> B[Check Fork]
    B --> C{Fork Active?}
    C -->|No| D[Build Block without VRF]
    C -->|Yes| E[Get Validators]
    E --> F[Collect VRF Keys]
    F --> G[Generate VRF]
    G --> H[Build Block with VRF]
    H --> I[Validate Header]
    I --> J{VRF Present?}
    J -->|No| K[Validate without VRF]
    J -->|Yes| L[Verify VRF Proofs]
    L --> M{Signer Selected?}
    M -->|No| N[Reject Block]
    M -->|Yes| O[Accept Block]
    O --> P[Compute State]
    P --> Q{Fork Active?}
    Q -->|No| R[Compute with All Validators]
    Q -->|Yes| S[Get VRF Proofs]
    S --> T[Select Validators]
    T --> U[Filter Votes]
    U --> V[Compute Quality]
    V --> W{Quality >= Threshold?}
    W -->|No| X[Not Finalized]
    W -->|Yes| Y[Finalized]
    X --> Z[Continue Waiting]
    Y --> AA[Success]
    N --> BB[Failure]
```

## Sequence Diagram

```mermaid
sequenceDiagram
    participant Packer as Packer
    participant Staker as Staker Contract
    participant VRF as VRF System
    participant Block as Block Builder
    participant Validator as Consensus Validator
    participant BFT as BFT Engine
    participant Chain as Blockchain

    Note over Packer,Chain: Phase 1: Block Generation
    Packer->>Packer: Check if fork HAYABUSA is active
    alt Fork not active
        Packer->>Block: Build block without VRF
        Block->>Chain: Submit block
    else Fork active
        Packer->>Staker: getValidatorsWithWeights()
        Staker-->>Packer: Return validator list with weights
        Note right of Packer: Whale has 40% stake, others 60%
        Packer->>Packer: collectValidatorVRFProofs()
        Packer->>VRF: WeightedValidatorSelection(privateKeys)
        VRF->>VRF: Generate VRF proofs for each validator
        Note right of VRF: VRF uses cryptographically secure randomness
        VRF->>VRF: selectValidatorsByWeight(beta, validators)
        Note right of VRF: Whale has 40% chance, others 60% chance
        VRF-->>Packer: Return VRF proofs and selected validators
        Note right of Packer: Selected validators may or may not include whale
        Packer->>Block: Builder.ValidatorVRFProofs(proofs).Build()
        Block->>Chain: Submit block with VRF proofs
    end

    Note over Validator,Chain: Phase 2: Block Validation
    Chain->>Validator: New block received
    Validator->>Validator: validateBlockHeader()
    alt VRF proofs present
        Validator->>Validator: validateWeightBasedVRF()
        Validator->>VRF: WeightedValidatorSelectionWithProofs(proofs)
        VRF->>VRF: Verify VRF proofs without private keys
        VRF->>VRF: Reconstruct selection using same beta
        VRF-->>Validator: Return selected validators
        Note right of Validator: Verify signer is in selected validators
        Validator->>Validator: Check if signer is in selected validators
        alt Signer selected
            Validator->>Chain: Accept block
        else Signer not selected
            Validator->>Chain: Reject block
        end
    else No VRF proofs
        Validator->>Chain: Accept block (legacy mode)
    end

    Note over BFT,Chain: Phase 3: Finality Computation (ANTI-WHALE MECHANISM)
    Chain->>BFT: Block accepted, compute finality
    BFT->>BFT: computeState()
    alt Fork active
        BFT->>BFT: computeStateWithVRF()
        BFT->>BFT: Extract VRF proofs from header
        BFT->>VRF: WeightedValidatorSelectionWithProofs(proofs)
        VRF->>VRF: Verify VRF proofs and reconstruct selection
        VRF-->>BFT: Return selected validators for finality
        Note right of BFT: CRITICAL: Only selected validators can vote for finality
        Note right of BFT: Whale cannot control finality if not selected
        BFT->>BFT: computeStateCommon(header, selectedValidators)
        Note right of BFT: Filter votes: only selected validators count
        Note right of BFT: Whale votes ignored if not selected by VRF
    else Fork not active
        BFT->>BFT: computeStateCommon(header, allValidators)
        Note right of BFT: Legacy mode: all validators can vote
    end
    BFT->>BFT: js.Summarize() - compute checkpoint quality
    Note right of BFT: Quality based only on selected validators
    alt Quality >= threshold
        BFT->>Chain: Block finalized
        Note right of BFT: Finality achieved without whale control
    else Quality < threshold
        BFT->>Chain: Continue waiting for more votes
        Note right of BFT: Whale cannot force finality if not selected
    end
```

## Anti-Whale Mechanism Explanation

### **Key Point: VRF Selection for Finality**
The critical anti-whale mechanism happens in **Phase 3** during finality computation:

1. **VRF Selection**: `vrf.WeightedValidatorSelectionWithProofs()` selects validators using cryptographically secure randomness
2. **Vote Filtering**: `computeStateCommon(header, selectedValidators)` only counts votes from VRF-selected validators
3. **Whale Prevention**: If a whale (40% stake) is not selected by VRF, their votes are completely ignored for finality

### **Example Scenario:**
- **Whale**: 40% stake, 40% chance of being selected
- **Other Validators**: 60% stake, 60% chance of being selected
- **If Whale Not Selected**: Whale cannot vote for finality, cannot block the process
- **If Whale Selected**: Whale can vote, but only with their proportional weight among selected validators

### **Security Guarantees:**
1. **No Whale Control**: Whale cannot guarantee they will be selected for finality
2. **Proportional Influence**: When selected, influence is proportional to stake among selected validators
3. **Verifiable Randomness**: Selection is cryptographically verifiable and unpredictable
4. **Decentralized Finality**: Finality depends on randomly selected validator subset

## Flow Explanation

### **Phase 1: Packer (Block Generation)**
1. **Check Fork**: `header.Number() >= forkConfig.HAYABUSA`
2. **Get Validators**: `getValidatorsWithWeights()` from `builtin.Staker.LeaderGroup()`
3. **Collect Keys**: `collectValidatorVRFProofs()` gets available private keys
4. **Generate VRF**: `vrf.WeightedValidatorSelection()` with private keys
5. **Build Block**: `Builder.ValidatorVRFProofs().Build()` includes VRF proofs

### **Phase 2: Consensus (Block Validation)**
1. **Validate Header**: `validateBlockHeader()` checks block structure
2. **Check VRF**: `header.ValidatorVRFProofs()` extracts VRF proofs
3. **Verify VRF**: `vrf.WeightedValidatorSelectionWithProofs()` validates proofs
4. **Check Signer**: `slices.Contains(selectedValidators, signer)` confirms selection

### **Phase 3: BFT Engine (Finality)**
1. **Compute State**: `computeState()` calculates BFT state
2. **VRF Filtering**: `computeStateWithVRF()` uses only VRF-selected validators
3. **Filter Votes**: `computeStateCommon(header, selectedSet)` filters validator votes
4. **Compute Quality**: `js.Summarize()` calculates checkpoint quality
5. **Finalization**: Quality threshold determines finality

### **VRF Components**
- **`vrf.Prove()`**: Generates beta (random hash) and pi (VRF proof)
- **`selectValidatorsByWeight()`**: Uses beta as seed for weighted selection
- **`vrf.WeightedValidatorSelectionWithProofs()`**: Validates VRF proofs without private keys

### **Security Benefits**
1. **Anti-Whale**: Whale cannot control finality if not selected
2. **Verifiable Randomness**: VRF provides cryptographically secure randomness
3. **Decentralization**: Voting power distributed proportionally to stake
4. **Backward Compatibility**: System works without VRF before fork 