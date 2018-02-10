pragma solidity ^0.4.18;

/// @title Authority manages the whitelist of block proposers.
contract Authority {

    function voting() public view returns(address) {
        return this.nativeGetVoting();
    }
    
    // @notice authorize someone to be a block proposer.
    // It will be reverted if someone already listed, 
    // @param _addr address of someone.
    // @param _identity identity to identify someone. Must be non-empty. 
    function authorize(address _addr, bytes32 _identity) public {
        require(msg.sender == this.nativeGetVoting());
        require(_addr != 0 && _identity != 0);

        require(this.nativeAddProposer(_addr, _identity));
        Authorize(_addr, _identity);
    }

    // @notice deauthorize a block proposer by its address.
    // @param _addr address of the proposer.
    function deauthorize(address _addr) public {
        require(msg.sender == this.nativeGetVoting());

        require(this.nativeRemoveProposer(_addr));
        Deauthorize(_addr);
    }

    function getProposer(address _addr) public view returns(bool found, bytes32 identity, uint32 status) {
        return this.nativeGetProposer(_addr);
    }

    function nativeGetVoting() public view returns(address) {}

    function nativeAddProposer(address addr, bytes32 identity) public returns(bool) {}
    function nativeRemoveProposer(address addr) public returns(bool) {}
    function nativeGetProposer(address addr) public view returns(bool, bytes32, uint32) {}

    // fired when an address authorized to be a proposer.
    event Authorize(address indexed addr, bytes32 identity);
    // fired when an address deauthorized.
    event Deauthorize(address indexed addr);
}

