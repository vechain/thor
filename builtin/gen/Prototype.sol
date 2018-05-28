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

    function $balanceAtBlock(address target, uint32 blockNumber) public view returns(uint256 amount){
        return  PrototypeNative(this).native_balanceAtBlock(target, blockNumber);
    }

    function $energyAtBlock(address target, uint32 blockNumber) public view returns(uint256 amount){
        return  PrototypeNative(this).native_energyAtBlock(target, blockNumber);
    }

    function $moveEnergyTo(address target, address to, uint256 amount) public{
        require($master(target) == msg.sender || target == msg.sender);
        PrototypeNative(this).native_moveEnergyTo(target, to, amount);
    }

    function $hasCode(address target) public view returns(bool){
        return PrototypeNative(this).native_hasCode(target);
    }

    function $storageAt(address target, bytes32 key) public view returns(bytes32 value){
        return PrototypeNative(this).native_storageAt(target, key);
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
    function native_moveEnergyTo(address target, address to, uint256 amount) public;
    function native_hasCode(address target) public view returns(bool);
    function native_storageAt(address target, bytes32 key) public view returns(bytes32 value);

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

    function $master(address receiver) public view returns(address master){
        return Prototype(prototypeContract).$master(receiver);
    }

    /// @param newMaster new master to be set.
    function $setMaster(address receiver, address newMaster) public {
        Prototype(prototypeContract).$setMaster(receiver, newMaster);
    }

    function $balanceAtBlock(address receiver, uint32 blockNumber) public view returns(uint256 amount){
        return Prototype(prototypeContract).$balanceAtBlock(receiver, blockNumber);
    }

    function $energy(address receiver) public view returns(uint256 amount){
        return Energy(energyContract).balanceOf(receiver);
    }

    function $energyAtBlock(address receiver, uint32 blockNumber) public view returns(uint256 amount){
        return Prototype(prototypeContract).$energyAtBlock(receiver, blockNumber);
    }

    function $transferEnergy(address receiver, uint256 amount) public{
        Energy(energyContract).transfer(receiver, amount);
    }

    function $moveEnergyTo(address receiver, address to, uint256 amount) public{
        Prototype(prototypeContract).$moveEnergyTo(receiver, to, amount);
    }

    function $hasCode(address receiver) public view returns(bool){
        return Prototype(prototypeContract).$hasCode(receiver);
    }

    function $storageAt(address receiver, bytes32 key) public view returns(bytes32 value){
        return Prototype(prototypeContract).$storageAt(receiver, key);
    }

    function $userPlan(address receiver) public view returns(uint256 credit, uint256 recoveryRate){
        return Prototype(prototypeContract).$userPlan(receiver);
    }

    function $setUserPlan(address receiver, uint256 credit, uint256 recoveryRate) public{
        Prototype(prototypeContract).$setUserPlan(receiver, credit, recoveryRate);
    }

    function $isUser(address receiver, address user) public view returns(bool){
        return Prototype(prototypeContract).$isUser(receiver, user);
    }

    function $userCredit(address receiver, address user) public view returns(uint256 remainedCredit){
        return Prototype(prototypeContract).$userCredit(receiver, user);
    }

    function $addUser(address receiver, address user) public{
        Prototype(prototypeContract).$addUser(receiver, user);
    }

    function $removeUser(address receiver, address user) public{
        Prototype(prototypeContract).$removeUser(receiver, user);
    }

    function $sponsor(address receiver, bool yesOrNo) public{
        Prototype(prototypeContract).$sponsor(receiver, yesOrNo);
    }

    function $isSponsor(address receiver, address sponsor) public view returns(bool){
        return Prototype(prototypeContract).$isSponsor(receiver, sponsor);
    }

    function $selectSponsor(address receiver, address sponsor) public{
        Prototype(prototypeContract).$selectSponsor(receiver, sponsor);
    }
    
    function $currentSponsor(address receiver) public view returns(address){
        return Prototype(prototypeContract).$currentSponsor(receiver);
    }

    event $SetMaster(address indexed newMaster);
    event $AddRemoveUser(address indexed user, bool addOrRemove);
    event $SetUserPlan(uint256 credit, uint256 recoveryRate);
    event $Sponsor(address indexed sponsor, bool yesOrNo);
    event $SelectSponsor(address indexed sponsor);

}