// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.4.24;

/// @title Prototype extends account.
contract Prototype {
    function master(address _self) public view returns(address){
        return PrototypeNative(this).native_master(_self);
    }

    function setMaster(address _self, address _newMaster) public {
        onlySelfOrMaster(_self);
        PrototypeNative(this).native_setMaster(_self, _newMaster);
    }

    function balance(address _self, uint _blockNumber) public view returns(uint256){
        return PrototypeNative(this).native_balanceAtBlock(_self, uint32(_blockNumber));
    }

    function energy(address _self, uint _blockNumber) public view returns(uint256){
        return PrototypeNative(this).native_energyAtBlock(_self, uint32(_blockNumber));
    }

    function hasCode(address _self) public view returns(bool){
        return PrototypeNative(this).native_hasCode(_self);
    }

    function storageFor(address _self, bytes32 _key) public view returns(bytes32){
        return PrototypeNative(this).native_storageFor(_self, _key);
    }

    function creditPlan(address _self) public view returns(uint256 credit, uint256 recoveryRate){
        return PrototypeNative(this).native_creditPlan(_self);
    }

    function setCreditPlan(address _self, uint256 _credit, uint256 _recoveryRate) public{
        onlySelfOrMaster(_self);
        PrototypeNative(this).native_setCreditPlan(_self, _credit, _recoveryRate);
    }

    function isUser(address _self, address _user) public view returns(bool){
        return PrototypeNative(this).native_isUser(_self, _user);
    }

    function userCredit(address _self, address _user) public view returns(uint256){
        return PrototypeNative(this).native_userCredit(_self, _user);
    }

    function addUser(address _self, address _user) public{
        require(_user != 0, "builtin: invalid user");
        onlySelfOrMaster(_self);
        require(PrototypeNative(this).native_addUser(_self, _user), "builtin: already added");
    }

    function removeUser(address _self, address _user) public{
        onlySelfOrMaster(_self);
        require(PrototypeNative(this).native_removeUser(_self, _user), "builtin: not a user");
    }

    function sponsor(address _self) public{
        require(PrototypeNative(this).native_sponsor(_self, msg.sender), "builtin: already sponsored");
    }

    function unsponsor(address _self) public {
        require(PrototypeNative(this).native_unsponsor(_self, msg.sender), "builtin: not sponsored");
    }

    function isSponsor(address _self, address _sponsor) public view returns(bool) {
        return PrototypeNative(this).native_isSponsor(_self, _sponsor);
    }

    function selectSponsor(address _self, address _sponsor) public {
        onlySelfOrMaster(_self);
        require(PrototypeNative(this).native_selectSponsor(_self, _sponsor), "builtin: not a sponsor");
    }

    function currentSponsor(address _self) public view returns(address){
        return PrototypeNative(this).native_currentSponsor(_self);
    }

    function onlySelfOrMaster(address _self) internal view {
        require(_self == msg.sender || PrototypeNative(this).native_master(_self) == msg.sender, "builtin: self or master required");
    }
}

contract PrototypeNative {
    function native_master(address self) public view returns(address);
    function native_setMaster(address self, address newMaster) public;

    function native_balanceAtBlock(address self, uint32 blockNumber) public view returns(uint256);
    function native_energyAtBlock(address self, uint32 blockNumber) public view returns(uint256);
    function native_hasCode(address self) public view returns(bool);
    function native_storageFor(address self, bytes32 key) public view returns(bytes32);

    function native_creditPlan(address self) public view returns(uint256, uint256);
    function native_setCreditPlan(address self, uint256 credit, uint256 recoveryRate) public;

    function native_isUser(address self, address user) public view returns(bool);
    function native_userCredit(address self, address user) public view returns(uint256);
    function native_addUser(address self, address user) public returns(bool);
    function native_removeUser(address self, address user) public returns(bool);

    function native_sponsor(address self, address sponsor) public returns(bool);
    function native_unsponsor(address self, address sponsor) public returns(bool);
    function native_isSponsor(address self, address sponsor) public view returns(bool);
    function native_selectSponsor(address self, address sponsor) public returns(bool);
    function native_currentSponsor(address self) public view returns(address);
}

contract PrototypeEvent {
    event $Master(address newMaster);
    event $CreditPlan(uint256 credit, uint256 recoveryRate);
    event $User(address indexed user, bytes32 action);
    event $Sponsor(address indexed sponsor, bytes32 action);
}
