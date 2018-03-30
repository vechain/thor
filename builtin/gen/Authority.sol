pragma solidity ^0.4.18;

/// @title Authority manages a candidates list of block proposers.
contract Authority {

    function executor() public view returns(address) {
        return this.nativeGetExecutor();
    }
    
    // @notice add a candidate of block proposer.
    // It will be reverted if it already listed, 
    // @param _signer address of the signer.
    // @param _endorsor address of endorsor that keeps certain amount of tokens. 
    // @param _identity identity of the candidate. Must be non-empty. 
    function add(address _signer, address _endorsor, bytes32 _identity) public {
        require(msg.sender == this.nativeGetExecutor());
        require(_signer != 0 && _endorsor != 0 && _identity != 0);

        require(this.nativeAdd(_signer, _endorsor, _identity));

        Add(_signer, _identity);
    }

    // @notice remove a candidate.
    // @param _signer address of the signer.
    function remove(address _signer) public {
        require(msg.sender == this.nativeGetExecutor() || !this.nativeIsEndorsed(_signer));

        require(this.nativeRemove(_signer));

        Remove(_signer);
    }

    function get(address _signer) public view returns(bool listed, address endorsor, bytes32 identity, bool active) {
        return this.nativeGet(_signer);
    }

    function first() public view returns(address) {
        return this.nativeFirst();
    }

    function next(address _signer) public view returns(address) {
        return this.nativeNext(_signer);
    }

    // fired when an address authorized to be a proposer.
    event Add(address indexed signer, bytes32 identity);
    // fired when an address deauthorized.
    event Remove(address indexed signer);


    function nativeGetExecutor() public view returns(address) {}
    function nativeAdd(address signer, address endorsor, bytes32 identity) public returns(bool) {}
    function nativeRemove(address signer) public returns(bool) {}
    function nativeGet(address signer) public view returns(bool, address, bytes32, bool) {}
    function nativeFirst() public view returns(address) {}
    function nativeNext(address signer) public view returns(address) {}
    function nativeIsEndorsed(address signer) public view returns(bool) {}
}
