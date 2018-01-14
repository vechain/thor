pragma solidity ^0.4.18;


/// @title Authority manages the whitelist of block proposers.
contract Authority {
    // fired when an address authorized to be a proposer.
    event Authorize(address indexed _addr, string _identity);
    // fired when an address deauthorized.
    event Deauthorize(address indexed _addr);

    // address of voting contract, which controls the whitelist.
    address public voting;

    using ProposerSet for ProposerSet.Type;
    using ProposerSet for bytes32;

    ProposerSet.Type proposerSet;
    mapping(address => string) identities;

    // @notice initialize the contract.
    // @param _voting the account controls this contract.
    function _initialize(address _voting) public {
        require(msg.sender == address(this));
        voting = _voting;
    }

    // @notice preset initial block proposers.
    // @param _addr address of proposer.
    // @param _identity identity of proposer.
    function _preset(address _addr, string _identity) public {
        require(msg.sender == address(this));
        require(bytes(_identity).length > 0);

        require(proposerSet.add(_addr));
        identities[_addr] = _identity;
    }

    // @notice udpate status of proposers.
    // @param _encoded the element is encoded as (address | (status << 160))
    function _update(bytes32[] _encoded) public {
        require(msg.sender == address(this));
        for (uint i = 0; i < _encoded.length; i++) {
            proposerSet.setStatus(_encoded[i].extractAddress(), _encoded[i].extractStatus());
        }        
    }

    // @notice authorize someone to be a block proposer.
    // It will be reverted if someone already listed, 
    // @param _addr address of someone.
    // @param _identity identity to identify someone. Must be non-empty. 
    function authorize(address _addr, string _identity) public {
        require(msg.sender == voting);
        require(bytes(_identity).length > 0);

        require(proposerSet.add(_addr));
        identities[_addr] = _identity;

        Authorize(_addr, _identity);
    }

    // @notice deauthorize a block proposer by its address.
    // @param _addr address of the proposer.
    function deauthorize(address _addr) public {
        require(msg.sender == voting);

        require(proposerSet.remove(_addr));
        delete identities[_addr];

        Deauthorize(_addr);
    }

    // @returns all block proposers.
    // The element is encoded as (address | (status << 160))
    function proposers() public view returns(bytes32[]) {
        return proposerSet.array;
    }

    // @param _addr address of proposer.
    // @returns proposer with the given `_addr`. Reverted if not found.
    function proposer(address _addr) public view returns(uint32 status, string identity) {
        return (proposerSet.getStatus(_addr), identities[_addr]);
    }
}

/// @title help to manage a set of proposers.
library ProposerSet {
    function encode(address _addr, uint32 _status) internal pure returns(bytes32) {
        return bytes32(_addr) | (bytes32(_status) << 160);
    }

    function extractAddress(bytes32 _encoded) internal pure returns(address) {
        return address(_encoded);
    }

    function extractStatus(bytes32 _encoded) internal pure returns(uint32) {
        return uint32(_encoded >> 160);
    }

    struct Type {
        // address -> (index + 1)
        mapping(address => uint) map;
        bytes32[] array;
    }

    function contains(Type storage _self, address _addr) internal view returns(bool) {
        return _self.map[_addr] != 0;
    }

    function add(Type storage _self, address _addr) internal returns(bool) {
        if (contains(_self, _addr))
            return false;
        
        _self.array.push(encode(_addr, 0));
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
        _self.map[extractAddress(_self.array[pos - 1])] = pos;

        _self.map[_addr] = 0;
        _self.array.length --;
        return true;
    }

    function setStatus(Type storage _self, address _addr, uint32 _status) internal {
        uint pos = _self.map[_addr];
        require(pos != 0);

        _self.array[pos - 1] = encode(_addr, _status);
    }

    function getStatus(Type storage _self, address _addr) internal view returns(uint32) {
        uint pos = _self.map[_addr];
        require(pos != 0);

        return extractStatus(_self.array[pos - 1]);
    }
}
