// Copyright (c) 2018 The VeChainThor developers
 
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity ^0.4.18;
import "./Energy.sol";

contract Prototype {

    /// @return master of account.
    /// For an external account, its master is initially zero.
    /// For a contract, its master is the msg sender of deployment.
    function $master(address target) public view returns(address master){
        return PrototypeNative(this).native_master(target);
    }

    /// @param newMaster new master to be set.
    function $setMaster(address target, address newMaster) public {
        require($master(target) == msg.sender || target == msg.sender);
        PrototypeNative(this).native_setMaster(target, newMaster);
    }

    function $balance(address target, uint32 blockNumber) public view returns(uint256 amount){
        return  PrototypeNative(this).native_balanceAtBlock(target, blockNumber);
    }

    function $energy(address target, uint32 blockNumber) public view returns(uint256 amount){
        return  PrototypeNative(this).native_energyAtBlock(target, blockNumber);
    }

    function $hasCode(address target) public view returns(bool){
        return PrototypeNative(this).native_hasCode(target);
    }

    function $storage(address target, bytes32 key) public view returns(bytes32 value){
        return PrototypeNative(this).native_storage(target, key);
    }

    function $storage(address target, bytes32 key, uint32 blockNumber) public view returns(bytes32 value){
        return PrototypeNative(this).native_storageAtBlock(target, key, blockNumber);
    }

    function $userPlan(address target) public view returns(uint256 credit, uint256 recoveryRate){
        return PrototypeNative(this).native_userPlan(target);
    }

    function $setUserPlan(address target, uint256 credit, uint256 recoveryRate) public{
        require($master(target) == msg.sender || target == msg.sender);
        PrototypeNative(this).native_setUserPlan(target, credit, recoveryRate);
    }

    function $isUser(address target, address user) public view returns(bool){
        return PrototypeNative(this).native_isUser(target, user);
    }

    function $userCredit(address target, address user) public view returns(uint256 remainedCredit){
        return PrototypeNative(this).native_userCredit(target, user);
    }

    function $addUser(address target, address user) public{
        require($master(target) == msg.sender || target == msg.sender);
        PrototypeNative(this).native_addUser(target, user);
    }

    function $removeUser(address target, address user) public{
        require($master(target) == msg.sender || target == msg.sender);
        PrototypeNative(this).native_removeUser(target, user);
    }

    function $sponsor(address target, bool yesOrNo) public{
        PrototypeNative(this).native_sponsor(target, msg.sender, yesOrNo);
    }

    function $isSponsor(address target, address sponsor) public view returns(bool){
        return PrototypeNative(this).native_isSponsor(target, sponsor);
    }

    function $selectSponsor(address target, address sponsor) public{
        require($master(target) == msg.sender || target == msg.sender);
        PrototypeNative(this).native_selectSponsor(target, sponsor);
    }
    
    function $currentSponsor(address target) public view returns(address){
        return PrototypeNative(this).native_currentSponsor(target);
    }

}

contract PrototypeNative {
    function native_master(address target) public view returns(address master);
    function native_setMaster(address target, address newMaster) public;

    function native_balanceAtBlock(address target, uint32 blockNumber) public view returns(uint256 amount);
    function native_energyAtBlock(address target, uint32 blockNumber) public view returns(uint256 amount);
    function native_hasCode(address target) public view returns(bool);
    function native_storage(address target, bytes32 key) public view returns(bytes32 value);
    function native_storageAtBlock(address target, bytes32 key, uint32 blockNumber) public view returns(bytes32 value);

    function native_userPlan(address target) public view returns(uint256 credit, uint256 recoveryRate);
    function native_setUserPlan(address target, uint256 credit, uint256 recoveryRate) public;

    function native_isUser(address target, address user) public view returns(bool);
    function native_userCredit(address target, address user) public view returns(uint256 remainedCredit);
    function native_addUser(address target, address user) public;
    function native_removeUser(address target, address user) public;

    function native_sponsor(address target, address caller, bool yesOrNo) public;
    function native_isSponsor(address target, address sponsor) public view returns(bool);
    function native_selectSponsor(address target, address sponsor) public;
    function native_currentSponsor(address target) public view returns(address);
}


library thor {

    address constant prototypeContract = uint160(bytes9("Prototype"));
    address constant energyContract = uint160(bytes6("Energy"));

    function $master(address receiver) internal view returns(address master){
        return Prototype(prototypeContract).$master(receiver);
    }

    /// @param newMaster new master to be set.
    function $setMaster(address receiver, address newMaster) internal {
        Prototype(prototypeContract).$setMaster(receiver, newMaster);
    }

    function $balance(address receiver, uint32 blockNumber) internal view returns(uint256 amount){
        return Prototype(prototypeContract).$balance(receiver, blockNumber);
    }

    function $energy(address receiver) internal view returns(uint256 amount){
        return Energy(energyContract).balanceOf(receiver);
    }

    function $energy(address receiver, uint32 blockNumber) internal view returns(uint256 amount){
        return Prototype(prototypeContract).$energy(receiver, blockNumber);
    }

    function $transferEnergy(address receiver, uint256 amount) internal{
        Energy(energyContract).transfer(receiver, amount);
    }

    function $moveEnergyTo(address receiver, address to, uint256 amount) internal{
        Energy(energyContract).move(receiver, to, amount);
    }

    function $hasCode(address receiver) internal view returns(bool){
        return Prototype(prototypeContract).$hasCode(receiver);
    }

    function $storage(address receiver, bytes32 key, uint32 blockNumber) internal view returns(bytes32 value){
        return Prototype(prototypeContract).$storage(receiver, key, blockNumber);
    }

    function $storage(address receiver, bytes32 key) internal view returns(bytes32 value){
        return Prototype(prototypeContract).$storage(receiver, key);
    }

    function $userPlan(address receiver) internal view returns(uint256 credit, uint256 recoveryRate){
        return Prototype(prototypeContract).$userPlan(receiver);
    }

    function $setUserPlan(address receiver, uint256 credit, uint256 recoveryRate) internal{
        Prototype(prototypeContract).$setUserPlan(receiver, credit, recoveryRate);
    }

    function $isUser(address receiver, address user) internal view returns(bool){
        return Prototype(prototypeContract).$isUser(receiver, user);
    }

    function $userCredit(address receiver, address user) internal view returns(uint256 remainedCredit){
        return Prototype(prototypeContract).$userCredit(receiver, user);
    }

    function $addUser(address receiver, address user) internal{
        Prototype(prototypeContract).$addUser(receiver, user);
    }

    function $removeUser(address receiver, address user) internal{
        Prototype(prototypeContract).$removeUser(receiver, user);
    }

    function $sponsor(address receiver, bool yesOrNo) internal{
        Prototype(prototypeContract).$sponsor(receiver, yesOrNo);
    }

    function $isSponsor(address receiver, address sponsor) internal view returns(bool){
        return Prototype(prototypeContract).$isSponsor(receiver, sponsor);
    }

    function $selectSponsor(address receiver, address sponsor) internal{
        Prototype(prototypeContract).$selectSponsor(receiver, sponsor);
    }
    
    function $currentSponsor(address receiver) internal view returns(address){
        return Prototype(prototypeContract).$currentSponsor(receiver);
    }

    event $SetMaster(address indexed newMaster);
    event $AddRemoveUser(address indexed user, bool addOrRemove);
    event $SetUserPlan(uint256 credit, uint256 recoveryRate);
    event $Sponsor(address indexed sponsor, bool yesOrNo);
    event $SelectSponsor(address indexed sponsor);

}