pragma solidity ^0.4.18;

import './Constants.sol';


/// @title Authority manages the whitelist of block proposers.
contract Authority {
    // fired when an address authorized to be a proposer.
    event Authorize(address indexed _addr, string _identity);
    // fired when an address deauthorized.
    event Deauthorize(address indexed _addr);

    struct Proposer {
        address addr;
        uint32 status;
        string identity;
    }

    // address of voting contract, which controls the whitelist.
    address public voting;

    using ProposerSet for ProposerSet.Type;

    ProposerSet.Type proposerSet;

    // @notice initialize the contract.
    function _initialize() public {
        require(msg.sender == address(this));
        voting = Constants.voting();
    }

    // @notice preset initial block proposers.
    // @param _addr address of proposer.
    // @param _identity identity of proposer.
    function _preset(address _addr, string _identity) public {
        require(msg.sender == address(this));
        require(bytes(_identity).length > 0);

        require(proposerSet.add(_addr, _identity));
    }

    // @notice udpate status of proposers.
    // input element is encoded as ((address << 32) | status).
    // @param _addrs addresses of proposers.
    // @param _status status of proposers corresponded `_addrs`.
    function _update(bytes24[] _encodedProposers) public {
        require(msg.sender == address(this));
        for (uint i = 0; i < _encodedProposers.length; i++) {
            address addr = address(_encodedProposers[i] >> 32);
            proposerSet.get(addr).status = uint32(_encodedProposers[i]);
        }        
    }

    // @notice authorize someone to be a block proposer.
    // It will be reverted if someone already listed, 
    // @param _addr address of someone.
    // @param _identity identity to identify someone. Must be non-empty. 
    function authorize(address _addr, string _identity) public {
        require(msg.sender == voting);
        require(bytes(_identity).length > 0);

        require(proposerSet.add(_addr, _identity));

        Authorize(_addr, _identity);
    }
    // @notice deauthorize a block proposer by its address.
    // @param _addr address of the proposer.
    function deauthorize(address _addr) public {
        require(msg.sender == voting);

        require(proposerSet.remove(_addr));

        Deauthorize(_addr);
    }

    // @returns all block proposers.
    // The element is encoded by ((address << 32) | status)
    function proposers() public view returns(bytes24[]) {
        var ret = new bytes24[](proposerSet.array.length);
        for (uint i = 0;i < proposerSet.array.length; i ++) {
            ret[i] = (bytes24(proposerSet.array[i].addr) << 32) |
                bytes24(proposerSet.array[i].status);
        }
        return ret;
    }

    // @param _addr address of proposer.
    // @returns proposer with the given `_addr`. Reverted if not found.
    function proposer(address _addr) public view returns(uint32 status, string identity) {
        var p = proposerSet.get(_addr);
        return (p.status, p.identity);
    }
}

/// @title help to manage a set of proposers.
library ProposerSet {
    struct Type {
        // address -> (index + 1)
        mapping(address => uint) map;
        Authority.Proposer[] array;
    }

    function contains(Type storage _self, address _addr) internal view returns(bool) {
        return _self.map[_addr] != 0;
    }

    function add(Type storage _self, address _addr, string _identity) internal returns(bool) {
        if (contains(_self, _addr))
            return false;

        _self.array.push(Authority.Proposer(_addr, 0, _identity));
        _self.map[_addr] = _self.array.length;
        return true;        
    }

    function remove(Type storage _self, address _addr) internal returns(bool) {
        uint pos = _self.map[_addr];
        if (pos == 0) {
            return false;            
        }

        // move last value to the gap
        _self.array[pos - 1] = _self.array[_self.array.length - 1];

        // remap
        _self.map[_self.array[pos - 1].addr] = pos;

        _self.map[_addr] = 0;
        _self.array.length --;
        return true;
    }

    function get(Type storage _self, address _addr) internal view returns(Authority.Proposer storage) {
        uint pos = _self.map[_addr];
        require(pos != 0);
        return _self.array[pos - 1];
    }
}
