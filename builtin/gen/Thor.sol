// Copyright (c) 2018 The VeChainThor developers
 
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity 0.4.24;
import "./Prototype.sol";
import "./Energy.sol";

library thor {

    address constant prototypeContract = uint160(bytes9("Prototype"));
    address constant energyContract = uint160(bytes6("Energy"));

    function $master(address self) internal view returns(address){
        return Prototype(prototypeContract).master(self);
    }

    /// @param newMaster new master to be set.
    function $setMaster(address self, address newMaster) internal {
        Prototype(prototypeContract).setMaster(self, newMaster);
    }

    function $balance(address self, uint blockNumber) internal view returns(uint256){
        return Prototype(prototypeContract).balance(self, blockNumber);
    }

    function $energy(address self) internal view returns(uint256 amount){
        return Energy(energyContract).balanceOf(self);
    }

    function $energy(address self, uint blockNumber) internal view returns(uint256){
        return Prototype(prototypeContract).energy(self, blockNumber);
    }

    function $transferEnergy(address self, uint256 amount) internal{
        Energy(energyContract).transfer(self, amount);
    }

    function $moveEnergyTo(address self, address to, uint256 amount) internal{
        Energy(energyContract).move(self, to, amount);
    }

    function $hasCode(address self) internal view returns(bool){
        return Prototype(prototypeContract).hasCode(self);
    }

    function $storage(address self, bytes32 key) internal view returns(bytes32){
        return Prototype(prototypeContract).storageFor(self, key);
    }

    function $userPlan(address self) internal view returns(uint256 credit, uint256 recoveryRate){
        return Prototype(prototypeContract).userPlan(self);
    }

    function $setUserPlan(address self, uint256 credit, uint256 recoveryRate) internal{
        Prototype(prototypeContract).setUserPlan(self, credit, recoveryRate);
    }

    function $isUser(address self, address user) internal view returns(bool){
        return Prototype(prototypeContract).isUser(self, user);
    }

    function $userCredit(address self, address user) internal view returns(uint256){
        return Prototype(prototypeContract).userCredit(self, user);
    }

    function $addUser(address self, address user) internal{
        Prototype(prototypeContract).addUser(self, user);
    }

    function $removeUser(address self, address user) internal{
        Prototype(prototypeContract).removeUser(self, user);
    }

    function $sponsor(address self, bool yesOrNo) internal{
        Prototype(prototypeContract).sponsor(self, yesOrNo);
    }

    function $isSponsor(address self, address sponsor) internal view returns(bool){
        return Prototype(prototypeContract).isSponsor(self, sponsor);
    }

    function $selectSponsor(address self, address sponsor) internal{
        Prototype(prototypeContract).selectSponsor(self, sponsor);
    }
    
    function $currentSponsor(address self) internal view returns(address){
        return Prototype(prototypeContract).currentSponsor(self);
    }

}