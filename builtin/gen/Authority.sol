// Copyright (c) 2018 The VeChainThor developers
 
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity ^0.4.18;

/// @title Authority manages a candidates list of block proposers.
contract Authority {

    function executor() public view returns(address) {
        return AuthorityNative(this).native_getExecutor();
    }
    
    // @notice add a candidate of block proposer.
    // It will be reverted if it already listed, 
    // @param _signer address of the signer.
    // @param _endorsor address of endorsor that keeps certain amount of tokens. 
    // @param _identity identity of the candidate. Must be non-empty. 
    function add(address _signer, address _endorsor, bytes32 _identity) public {
        require(msg.sender == executor());
        require(_signer != 0 && _endorsor != 0 && _identity != 0);

        require(AuthorityNative(this).native_add(_signer, _endorsor, _identity));

        emit Add(_signer, _endorsor, _identity);
    }

    // @notice remove a candidate.
    // @param _signer address of the signer.
    function remove(address _signer) public {
        require(msg.sender == executor() || !AuthorityNative(this).native_isEndorsed(_signer));

        require(AuthorityNative(this).native_remove(_signer));

        emit Remove(_signer);
    }

    function get(address _signer) public view returns(bool listed, address endorsor, bytes32 identity, bool active) {
        return AuthorityNative(this).native_get(_signer);
    }

    function first() public view returns(address) {
        return AuthorityNative(this).native_first();
    }

    function next(address _signer) public view returns(address) {
        return AuthorityNative(this).native_next(_signer);
    }

    // fired when an address authorized to be a proposer.
    event Add(address indexed signer, address endorsor, bytes32 identity);
    // fired when an address deauthorized.
    event Remove(address indexed signer);    
}


contract AuthorityNative {
    function native_getExecutor() public view returns(address);
    function native_add(address signer, address endorsor, bytes32 identity) public returns(bool);
    function native_remove(address signer) public returns(bool);
    function native_get(address signer) public view returns(bool, address, bytes32, bool);
    function native_first() public view returns(address);
    function native_next(address signer) public view returns(address);
    function native_isEndorsed(address signer) public view returns(bool);
}