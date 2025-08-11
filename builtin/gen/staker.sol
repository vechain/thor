//SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

struct Node {
    address next;
    address prev;
}

enum ValidatorStatus {
    Unknown,
    Queued,
    Active,
    Exited
}

struct Validation {
    // ---- Slot 0 ----
    address endorsor; // 20 bytes
    uint32 period; // 4 bytes
    uint32 completeIterations; // 4 bytes
    ValidatorStatus status; // 1 byte
    bool online; // 1 byte
    // ---- Slot 1 ----
    uint32 startBlock; // 4 bytes
    uint32 exitBlock; // 4 bytes
    // ---- Slots 2–7 ---- (each one uint256)
    uint256 lockedVET;
    uint256 pendingUnlockVET;
    uint256 queuedVET;
    uint256 cooldownVET;
    uint256 withdrawableVET;
    uint256 weight;
}

struct Aggregation {
    uint256 lockedVET; // VET locked this period (autoRenew == true)
    uint256 lockedWeight; // Weight including multipliers
    uint256 pendingVET; // VET that is pending to be locked in the next period (autoRenew == false)
    uint256 pendingWeight; // Weight including multipliers
    uint256 exitingVET; // VET that is exiting the next period
    uint256 exitingWeight; // Weight including multipliers
}

struct Config {
    uint32 epochLength; // in blocks
    uint32 cooldownPeriod; // in blocks
    uint32 lowStakePeriodLength; // in blocks
    uint32 midStakePeriodLength; // in blocks
    uint32 highStakePeriodLength; // in blocks
}

struct Delegation {
    // ---- Slot 0 ----
    address validator; // 20 bytes
    uint8 multiplier; // 1 byte
    uint32 firstIteration; // 4 bytes
    uint32 lastIteration; // 4 bytes
    // total: 29 bytes (fits in one slot)

    // ---- Slot 1 ----
    uint256 stake; // full 32 bytes
}

contract Staker {
    uint256 public constant MAX_STAKE = 600e6 ether; // 600 million VET
    uint256 public constant MIN_STAKE = 1e18 ether; // 1 VET
    uint8 public constant VALIDATOR_MULTIPLIER = 200;

    // slot 0 - config valiables
    Config private _config =
        Config({
            epochLength: 180,
            cooldownPeriod: 360 * 24, // 24 hours in blocks;
            lowStakePeriodLength: 360 * 24 * 7, // 7 days in blocks
            midStakePeriodLength: 360 * 24 * 15, // 15 days in blocks
            highStakePeriodLength: 360 * 24 * 30 // 30 days in blocks
        });

    // slot 1
    uint256 private _lockedVET;
    // slot 2
    uint256 private _lockedWeight;
    // slot 3
    uint256 private _queuedVET;
    // slot 4
    uint256 private _queuedWeight;

    // slot 5
    mapping(address => Node) private _queuedValidators;
    // slot 6
    address private _firstQueuedValidator;
    // slot 7
    address private _lastQueuedValidator;
    // slot 8
    uint256 private _queuedValidatorsCount;

    // slot 9
    mapping(address => Node) private _activeValidators;
    // slot 10
    address private _firstActiveValidator;
    // slot 11
    address private _lastActiveValidator;
    // slot 12
    uint256 private _activeValidatorsCount;

    // slot 13
    mapping(address => Validation) private _validators;
    // slot 14
    mapping(uint32 => address) private _exitBlocks;
    // slot 15
    mapping(address => Aggregation) private _aggregations;

    // slot 16
    mapping(uint256 => Delegation) private _delegations;
    // slot 17
    uint256 private _delegationCounter;
    // slot 18
    mapping(bytes32 => uint256) private _delegatorsRewards;

    event ValidationQueued(
        address indexed validator,
        address indexed endorsor,
        uint32 period,
        uint256 stake
    );
    event ValidationWithdrawn(address indexed validator, uint256 stake);
    event ValidationSignaledExit(address indexed validator);
    event StakeIncreased(address indexed validator, uint256 added);
    event StakeDecreased(address indexed validator, uint256 removed);

    event DelegationAdded(
        address indexed validator,
        uint256 indexed delegationID,
        uint256 stake,
        uint8 multiplier
    );
    event DelegationWithdrawn(uint256 indexed delegationID, uint256 stake);
    event DelegationSignaledExit(uint256 indexed delegationID);

    function totalStake() public view returns (uint256, uint256) {
        return (_lockedVET, _lockedWeight);
    }

    function queuedStake() public view returns (uint256, uint256) {
        return (_queuedVET, _queuedWeight);
    }

    function addValidation(
        address validator,
        uint32 period
    ) public payable checkStake(msg.value) {
        require(
            period == _config.lowStakePeriodLength ||
                period == _config.midStakePeriodLength ||
                period == _config.highStakePeriodLength,
            "invalid staking period"
        );
        require(
            _validators[validator].status == ValidatorStatus.Unknown,
            "validator already registered"
        );
        require(msg.value <= MAX_STAKE, "stake exceeds maximum allowed");
        require(msg.value >= MIN_STAKE, "stake is below minimum required");

        // store the validator
        Validation storage v = _validators[validator];
        v.endorsor = msg.sender;
        v.period = period;
        v.status = ValidatorStatus.Queued;
        v.queuedVET = msg.value;
        v.exitBlock = type(uint32).max; // set to max, will be updated later
        _validators[validator] = v;

        // add to the queue
        _addToQueue(validator);
        // update the total queued stake
        _addQueuedStake(msg.value, VALIDATOR_MULTIPLIER);
        emit ValidationQueued(validator, msg.sender, period, msg.value);
    }

    function increaseStake(
        address validator
    )
        public
        payable
        checkStake(msg.value)
        checkMaxStake(validator, msg.value)
        onlyEndorsor(validator)
        queuedOrActive(validator)
    {
        _validators[validator].queuedVET += msg.value;
        _addQueuedStake(msg.value, VALIDATOR_MULTIPLIER);
        emit StakeIncreased(validator, msg.value);
    }

    function decreaseStake(
        address validator,
        uint256 amount
    )
        public
        checkStake(amount)
        checkMinStake(validator, amount)
        onlyEndorsor(validator)
        queuedOrActive(validator)
    {
        if (_validators[validator].status == ValidatorStatus.Queued) {
            _validators[validator].queuedVET -= amount;
            _validators[validator].withdrawableVET += amount;
            _removeQueuedStake(amount, VALIDATOR_MULTIPLIER);
        } else {
            _validators[validator].pendingUnlockVET += amount;
        }
        emit StakeDecreased(validator, amount);
    }

    function withdrawStake(address validator) public onlyEndorsor(validator) {
        uint256 queued = _validators[validator].queuedVET;
        uint256 withdrawable = _validators[validator].withdrawableVET + queued;

        _removeQueuedStake(queued, VALIDATOR_MULTIPLIER);

        _validators[validator].queuedVET = 0;
        _validators[validator].withdrawableVET = 0;

        if (_validators[validator].status == ValidatorStatus.Queued) {
            _validators[validator].status = ValidatorStatus.Exited; // move the validator to exited status
            _removeFromQueue(validator); // remove from the queue
        } else if (_hasPassedCooldown(validator)) {
            withdrawable += _validators[validator].cooldownVET;
            _validators[validator].cooldownVET = 0;
        }

        _validators[validator].withdrawableVET = 0;

        // send the withdrawable amount
        (bool success, ) = msg.sender.call{value: withdrawable}("");
        require(success, "withdraw failed");
        emit ValidationWithdrawn(validator, withdrawable);
    }

    function signalExit(address validator) public onlyEndorsor(validator) {
        require(
            _validators[validator].status == ValidatorStatus.Active,
            "validator not active"
        );

        uint32 minExitBlock = _validators[validator].startBlock +
            _validators[validator].period *
            (_validators[validator].completeIterations + 1);

        _validators[validator].exitBlock = _setExitBlock(
            validator,
            minExitBlock
        );
        emit ValidationSignaledExit(validator);
    }

    function addDelegation(
        address validator,
        uint8 multiplier // (% of msg.value) 100 for x1, 200 for x2, etc. This enforces a maximum of 2.56x multiplier
    )
        public
        payable
        onlyDelegatorContract
        checkStake(msg.value)
        queuedOrActive(validator)
        checkMaxStake(validator, msg.value)
        returns (uint256)
    {
        require(multiplier != 0, "multiplier cannot be zero");

        require(multiplier <= 256, "multiplier cannot exceed 256 (2.56x)"); // 256 is the maximum multiplier allowed

        require(
            _validators[validator].status == ValidatorStatus.Active,
            "validator not active"
        );

        require(
            _validators[validator].endorsor != msg.sender,
            "delegator cannot be the endorsor"
        );

        // create a new delegation
        _delegationCounter++;
        Delegation storage delegation = _delegations[_delegationCounter];
        delegation.validator = validator;
        delegation.multiplier = multiplier;
        delegation.stake = msg.value;
        delegation.lastIteration = type(uint32).max;

        if (_validators[validator].status == ValidatorStatus.Queued) {
            delegation.firstIteration = 1;
        } else {
            delegation.firstIteration =
                _validators[validator].completeIterations +
                2;
        }

        _aggregations[validator].pendingVET += msg.value;
        _aggregations[validator].pendingWeight +=
            (msg.value * multiplier) /
            100;
        _addQueuedStake(msg.value, multiplier);

        emit DelegationAdded(
            validator,
            _delegationCounter,
            msg.value,
            multiplier
        );
        return _delegationCounter;
    }

    function signalDelegationExit(
        uint256 delegationID
    ) public onlyDelegatorContract {
        require(_delegations[delegationID].stake > 0, "delegation not found");
        require(
            _delegations[delegationID].lastIteration == type(uint32).max,
            "delegation already exiting or exited"
        );
        address validator = _delegations[delegationID].validator;

        _delegations[delegationID].lastIteration = _validatorNextPeriod(
            validator
        );
        _aggregations[validator].exitingVET += _delegations[delegationID].stake;
        _aggregations[validator].exitingWeight +=
            (_delegations[delegationID].stake *
                _delegations[delegationID].multiplier) /
            100; // Convert percentage to multiplier

        emit DelegationSignaledExit(delegationID);
    }

    function withdrawDelegation(
        uint256 delegationID
    ) public onlyDelegatorContract {
        require(_delegations[delegationID].stake > 0, "delegation not found");

        address validator = _delegations[delegationID].validator;
        bool started = _hasDelegationStarted(delegationID, validator);

        require(
            _isDelegationLocked(delegationID, validator) == false,
            "delegation is locked"
        );

        uint256 stake = _delegations[delegationID].stake;
        uint8 multiplier = _delegations[delegationID].multiplier;

        if (!started) {
            _aggregations[validator].pendingVET -= _delegations[delegationID]
                .stake;
            _aggregations[validator].pendingWeight -= _calcWeight(
                stake,
                multiplier
            );
            _removeQueuedStake(stake, multiplier);
        }

        _delegations[delegationID].stake = 0;

        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "withdraw failed");
        emit DelegationWithdrawn(delegationID, stake);
    }

    function getDelegationStake(
        uint256 delegationID
    ) public view returns (address, uint256, uint8) {
        return (
            _delegations[delegationID].validator,
            _delegations[delegationID].stake,
            _delegations[delegationID].multiplier
        );
    }

    function getDelegationPeriodDetails(
        uint256 delegationID
    ) public view returns (uint32, uint32, bool) {
        return (
            _delegations[delegationID].firstIteration,
            _delegations[delegationID].lastIteration,
            _isDelegationLocked(
                delegationID,
                _delegations[delegationID].validator
            )
        );
    }

    function getValidatorStake(
        address validator
    ) public view returns (address, uint256, uint256, uint256) {
        Validation storage v = _validators[validator];
        return (v.endorsor, v.lockedVET, v.weight, v.queuedVET);
    }

    function getValidatorStatus(
        address validator
    ) public view returns (uint8, bool) {
        return (
            uint8(_validators[validator].status),
            _validators[validator].online
        );
    }

    function getValidatorPeriodDetails(
        address validator
    ) public view returns (uint32, uint32, uint32, uint32) {
        Validation storage v = _validators[validator];
        return (v.period, v.startBlock, v.exitBlock, v.completeIterations);
    }

    function getWithdrawable(address id) public view returns (uint256) {
        uint256 withdrawable = _validators[id].withdrawableVET;
        if (_validators[id].status == ValidatorStatus.Queued) {
            withdrawable += _validators[id].queuedVET;
        } else if (_hasPassedCooldown(id)) {
            withdrawable += _validators[id].cooldownVET;
        }
        return withdrawable;
    }

    function firstActive() public view returns (address) {
        return _firstActiveValidator;
    }

    function firstQueued() public view returns (address) {
        return _firstQueuedValidator;
    }

    function next(address prev) public view returns (address) {
        address activeNext = _activeValidators[prev].next;
        if (activeNext != address(0)) {
            return activeNext;
        }
        return _queuedValidators[prev].next;
    }

    function getDelegatorsRewards(
        address validator,
        uint32 stakingPeriod
    ) public view returns (uint256) {
        bytes32 key = keccak256(abi.encodePacked(validator, stakingPeriod));
        return _delegatorsRewards[key];
    }

    function getValidationTotals(
        address validator
    )
        public
        view
        returns (uint256, uint256, uint256, uint256, uint256, uint256)
    {
        Validation storage v = _validators[validator];
        Aggregation storage agg = _aggregations[validator];

        uint256 exitingVET;
        uint256 exitingWeight;

        if (v.status == ValidatorStatus.Active && v.exitBlock != 0) {
            exitingVET = v.lockedVET + agg.lockedVET;
            exitingWeight = v.weight;
        } else {
            exitingVET = v.pendingUnlockVET + agg.exitingVET;
            exitingWeight = v.weight + agg.exitingWeight;
        }

        uint256 queuedVET = v.queuedVET + agg.pendingVET;
        uint256 queuedWeight = _calcWeight(v.queuedVET, VALIDATOR_MULTIPLIER) +
            agg.pendingWeight;

        return (
            v.lockedVET + agg.lockedVET,
            v.weight,
            queuedVET,
            queuedWeight,
            exitingVET,
            exitingWeight
        );
    }

    function getValidatorsNum() public view returns (uint256, uint256) {
        return (_activeValidatorsCount, _queuedValidatorsCount);
    }

    function issuance() public view returns (uint256) {
        (uint256 issuanceAmount, string memory error) = StakerNative(
            address(this)
        ).native_issuance();
        require(bytes(error).length == 0, error);
        return issuanceAmount;
    }

    function _addToQueue(address validator) internal {
        Node storage node = _queuedValidators[validator];
        node.prev = _lastQueuedValidator;
        _queuedValidators[validator] = node;
        if (_queuedValidatorsCount == 0) {
            _firstQueuedValidator = validator;
        } else {
            _queuedValidators[_lastQueuedValidator].next = validator;
        }
        _lastQueuedValidator = validator;
        _queuedValidatorsCount++;
    }

    function _removeFromQueue(address validator) internal {
        Node storage node = _queuedValidators[validator];
        if (node.prev != address(0)) {
            _queuedValidators[node.prev].next = node.next;
        } else {
            _firstQueuedValidator = node.next; // update first if this was the first
        }
        if (node.next != address(0)) {
            _queuedValidators[node.next].prev = node.prev;
        } else {
            _lastQueuedValidator = node.prev; // update last if this was the last
        }
        delete _queuedValidators[validator];
        _queuedValidatorsCount--;
    }

    function _setExitBlock(
        address validator,
        uint32 minBlock
    ) internal returns (uint32) {
        uint32 exitBlock = minBlock;
        while (_exitBlocks[exitBlock] != address(0)) {
            exitBlock = exitBlock + _config.epochLength;
        }
        _exitBlocks[exitBlock] = validator;
        return exitBlock;
    }

    function _hasPassedCooldown(
        address validator
    ) internal view returns (bool) {
        return
            _validators[validator].status == ValidatorStatus.Exited && // validator has exited
            block.number >=
            _validators[validator].exitBlock + _config.cooldownPeriod; // cooldown period has passed
    }

    function _addQueuedStake(uint256 amount, uint8 multiplier) internal {
        _queuedVET += amount;
        _queuedWeight += _calcWeight(amount, multiplier);
    }

    function _removeQueuedStake(uint256 amount, uint8 multiplier) internal {
        _queuedVET -= amount;
        _queuedWeight -= _calcWeight(amount, multiplier);
    }

    function _validatorNextPeriod(
        address validator
    ) internal view returns (uint32) {
        if (_validators[validator].status == ValidatorStatus.Queued) {
            return 1;
        }

        // +2, 1 for the current period, 1 for the next
        return _validators[validator].completeIterations + 2;
    }

    function _calcWeight(
        uint256 amount,
        uint8 multiplier
    ) internal pure returns (uint256) {
        return (amount * multiplier) / 100; // Convert percentage to multiplier
    }

    function _isDelegationLocked(
        uint256 delegationID,
        address validator
    ) internal view returns (bool) {
        bool started = _hasDelegationStarted(delegationID, validator);
        bool ended = _hasDelegationEnded(delegationID, validator);
        return started && !ended;
    }

    function _hasDelegationStarted(
        uint256 delegationID,
        address validator
    ) internal view returns (bool) {
        return
            _delegations[delegationID].firstIteration >
            _validators[validator].completeIterations;
    }

    function _hasDelegationEnded(
        uint256 delegationID,
        address validator
    ) internal view returns (bool) {
        return
            _delegations[delegationID].lastIteration <=
            _validators[validator].completeIterations;
    }

    modifier checkMaxStake(address validator, uint256 amount) {
        uint256 validatorNextPeriodTVL = _validators[validator].queuedVET +
            _validators[validator].lockedVET -
            _validators[validator].pendingUnlockVET;
        uint256 aggregationNextPeriodTVL = _aggregations[validator].pendingVET +
            _aggregations[validator].lockedVET -
            _aggregations[validator].exitingVET;
        uint256 totalNextPeriodTVL = validatorNextPeriodTVL +
            aggregationNextPeriodTVL +
            amount;
        require(
            totalNextPeriodTVL <= MAX_STAKE,
            "stake exceeds maximum allowed for validation"
        );
        _;
    }

    modifier onlyEndorsor(address validator) {
        require(
            _validators[validator].endorsor == msg.sender,
            "not endorsor of the validator"
        );
        _;
    }

    modifier queuedOrActive(address validator) {
        require(
            _validators[validator].status == ValidatorStatus.Queued ||
                _validators[validator].status == ValidatorStatus.Active,
            "validator not queued or active"
        );
        _;
    }

    modifier onlyDelegatorContract() {
        require(
            msg.sender ==
                StakerNative(address(this)).native_delegatorContract(),
            "builtin: only delegator"
        );
        _;
    }

    modifier checkStake(uint256 amount) {
        require(amount > 0, "stake is empty");
        require(amount % 1e18 == 0, "stake is not multiple of 1VET");
        _;
    }

    modifier checkMinStake(address validator, uint256 amount) {
        ValidatorStatus status = _validators[validator].status;
        if (status == ValidatorStatus.Queued) {
            require(
                _validators[validator].queuedVET - amount >= MIN_STAKE,
                "not enough queued stake"
            );
        }

        if (status == ValidatorStatus.Active) {
            uint256 currentNextPeriod = _validators[validator].lockedVET -
                _validators[validator].pendingUnlockVET;
            require(
                currentNextPeriod - amount >= MIN_STAKE,
                "not enough locked stake"
            );
        }
        _;
    }

    receive() external payable {
        revert("receive function not allowed");
    }

    fallback() external {
        revert("fallback function not allowed");
    }
}

interface StakerNative {
    function native_issuance() external view returns (uint256, string calldata);
    function native_delegatorContract() external view returns (address);
}
