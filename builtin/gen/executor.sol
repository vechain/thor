// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.4.24;

interface _protocol_subset {
    function addUser(address _self, address _user) external;
    function removeUser(address _self, address _user) external;
}

library _proto_helper {
    _protocol_subset constant proto = _protocol_subset(uint160(bytes9("Prototype")));
    function $addUser(address _self, address _user) internal {
        proto.addUser(_self, _user);
    }

    function $removeUser(address _self, address _user) internal {
        proto.removeUser(_self, _user);
    }
}

/// @title Executor core component for on-chain governance.
contract Executor {
    using _proto_helper for Executor;
    struct proposal{
        uint64 timeProposed;    // when the proposal was raised        
        address proposer;       // usually a voting contract address
        uint8 quorum;           // min approval count for execution
        uint8 approvalCount;    // current approval count
        bool executed;          // whether the proposal has been executed
        // content of proposal: target contract address and call data
        address target;
        bytes data;
        mapping(address => bool) approvals;
    }

    struct approver {
        bytes32 identity;
        bool inPower;
    }

    mapping(address => approver) public approvers;
    uint8 public approverCount;
    mapping(address => bool) public votingContracts;
    mapping(bytes32 => proposal) public proposals;

    function propose(address _target, bytes _data) public returns(bytes32) {
        require(_target != 0, "builtin: invalid target");
        require(approverCount > 0, "builtin: no approvers");
        require(approvers[msg.sender].inPower || votingContracts[msg.sender], "builtin: approver or voting contract required");

        bytes32 proposalID = keccak256(abi.encodePacked(uint64(now), msg.sender));
        require(proposals[proposalID].timeProposed == 0, "builtin: duplicated proposal id");

        proposals[proposalID] = proposal(
            uint64(now),
            msg.sender,
            quorum(approverCount),
            0,
            false,
            _target,
            _data
        );

        emit Proposal(proposalID, "proposed");
        return proposalID;
    }

    function approve(bytes32 _proposalID) public {
        require(proposals[_proposalID].timeProposed > 0, "builtin: proposal not found");
        require(approvers[msg.sender].inPower, "builtin: approver required");
        require(now - proposals[_proposalID].timeProposed < 1 weeks, "builtin: proposal expired");
        require(!proposals[_proposalID].approvals[msg.sender], "builtin: proposal approved");

        proposals[_proposalID].approvals[msg.sender] = true;
        proposals[_proposalID].approvalCount++;

        emit Proposal(_proposalID, "approved");
    }

    function execute(bytes32 _proposalID) public {
        require(proposals[_proposalID].timeProposed > 0, "builtin: proposal not found");
        require(!proposals[_proposalID].executed, "builtin: proposal executed");
        require(now - proposals[_proposalID].timeProposed < 1 weeks, "builtin: proposal expired");
        require(proposals[_proposalID].approvalCount >= proposals[_proposalID].quorum, "builtin: quorum unsatisfied");

        proposals[_proposalID].executed = true; // set before call to prevent re-enter attack
        require(proposals[_proposalID].target.call(proposals[_proposalID].data), "builtin: proposal execution reverted");

        emit Proposal(_proposalID, "executed");
    }

    function addApprover(address _approver, bytes32 _identity) public {
        onlyThis();
        require(_approver != 0, "builtin: invalid approver");
        require(_identity != 0, "builtin: invalid identity");
        require(approvers[_approver].identity == 0, "builtin: approver exists");
        require(approverCount < 255, "builtin: too many approvers");

        approvers[_approver] = approver(_identity, true);
        approverCount++;
        this.$addUser(_approver);
        emit Approver(_approver, "added");
    }

    function revokeApprover(address _approver) public {
        onlyThis();
        require(approvers[_approver].inPower, "builtin: unknown or revoked approver");

        approvers[_approver].inPower = false;
        approverCount--;
        this.$removeUser(_approver);
        emit Approver(_approver, "revoked");
    }

    function attachVotingContract(address _contract) public {
        onlyThis();
        require(_contract != 0, "builtin: invalid contract address");
        require(!votingContracts[_contract], "builtin: voting contract already attached");

        votingContracts[_contract] = true;

        emit VotingContract(_contract, "attached");
    }

    function detachVotingContract(address _contract) public {
        onlyThis();
        require(votingContracts[_contract], "builtin: voting contract not attached");

        votingContracts[_contract] = false;

        emit VotingContract(_contract, "detached");
    }

    function onlyThis() internal view {
        require(msg.sender == address(this), "builtin: executor required");
    }

    function quorum(uint8 total) internal pure returns(uint8) {
        return uint8((uint(total) + 1) * 2 / 3);
    }
    
    event Proposal(bytes32 indexed proposalID, bytes32 action);
    event Approver(address indexed approver, bytes32 action);
    event VotingContract(address indexed contractAddr, bytes32 action);
}
