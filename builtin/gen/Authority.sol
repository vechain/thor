pragma solidity ^0.4.18;

/// @title Authority manages the whitelist of block proposers.
contract Authority {

    function n() internal view returns(AuthorityNative) {
        return AuthorityNative(this);
    }

    function executor() public view returns(address) {
        return n().nativeGetExecutor();
    }
    
    // @notice authorize someone to be a block proposer.
    // It will be reverted if someone already listed, 
    // @param _addr address of someone.
    // @param _identity identity to identify someone. Must be non-empty. 
    function authorize(address _addr, bytes32 _identity) public {
        require(msg.sender == n().nativeGetExecutor());
        require(_addr != 0 && _identity != 0);

        require(n().nativeAdd(_addr, _identity));

        Authorize(_addr, _identity);
    }

    // @notice deauthorize a block proposer by its address.
    // @param _addr address of the proposer.
    function deauthorize(address _addr) public {
        require(msg.sender == n().nativeGetExecutor());

        require(n().nativeRemove(_addr));

        Deauthorize(_addr);
    }

    function status(address _addr) public view returns(bool listed, bytes32 identity, uint32) {
        return n().nativeStatus(_addr);
    }

    function count() public view returns(uint64) {
        return n().nativeCount();
    }

    // fired when an address authorized to be a proposer.
    event Authorize(address indexed addr, bytes32 identity);
    // fired when an address deauthorized.
    event Deauthorize(address indexed addr);
}

contract AuthorityNative {
    function nativeGetExecutor() public view returns(address);

    function nativeAdd(address addr, bytes32 identity) public returns(bool);
    function nativeRemove(address addr) public returns(bool);
    function nativeStatus(address addr) public view returns(bool, bytes32, uint32);
    function nativeCount() public view returns(uint64);
}