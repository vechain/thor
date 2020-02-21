// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.4.24;
import "./authority.sol";

/// @title Authority manages a candidates list of master nodes(block proposers).
contract AuthorityV2 is Authority {
    function add2(address _nodeMaster, address _endorsor, bytes32 _identity, bytes32 _vrfPublicKey) public {
        require(_nodeMaster != 0, "builtin: invalid node master");
        require(_endorsor != 0, "builtin: invalid endorsor");
        require(_identity != 0, "builtin: invalid identity");
        require(_vrfPublicKey != 0, "builtin: invalid vrf public key");
        require(msg.sender == executor(), "builtin: executor required");

        require(AuthorityV2Native(this).native_add2(_nodeMaster, _endorsor, _identity, _vrfPublicKey), "builtin: already exists");

        emit Candidate(_nodeMaster, "added");
    }

    function get2(address _nodeMaster) public view returns(bool listed, address endorsor, bytes32 identity, bool active, bytes32 vrfPublicKey) {
        return AuthorityV2Native(this).native_get2(_nodeMaster);
    }
}

contract AuthorityV2Native is AuthorityNative {
    function native_add2(address nodeMaster, address endorsor, bytes32 identity, bytes32 vrfPublicKey) public returns(bool);
    function native_get2(address nodeMaster) public view returns(bool, address, bytes32, bool, bytes32);
}
