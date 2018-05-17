// Copyright (c) 2018 The VeChainThor developers
 
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

pragma solidity ^0.4.18;

contract Prototype {
    function Of(address addr) public view returns(PrototypeInterface) {
        PrototypeNative(this).native_contractify(addr);
        return PrototypeInterface(addr);
    }
}

contract PrototypeNative {
    function native_contractify(address addr) public view;
}

contract PrototypeInterface {

    /// @return master of account.
    /// For an external account, its master is initially zero.
    /// For a contract, its master is the msg sender of deployment.
    function $master() public view returns(address master);

    /// @param newMaster new master to be set.
    function $set_master(address newMaster) public;

    function $has_code() public view returns(bool);

    function $energy() public view returns(uint256);
    function $transfer_energy(uint256 amount) public;
    function $move_energy_to(address to, uint256 amount) public;

    function $user_plan() public view returns(uint256 credit, uint256 recoveryRate);
    function $set_user_plan(uint256 credit, uint256 recoveryRate) public;

    function $is_user(address user) public view returns(bool);
    function $user_credit(address user) public view returns(uint256 remainedCredit);
    function $add_user(address user) public;
    function $remove_user(address user) public;

    function $sponsor(bool yesOrNo) public;
    function $is_sponsor(address sponsor) public view returns(bool);
    function $select_sponsor(address sponsor) public;
    function $current_sponsor() public view returns(address);

    event $SetMaster(address indexed newMaster);
    event $AddRemoveUser(address indexed user, bool addOrRemove);
    event $SetUserPlan(uint256 credit, uint256 recoveryRate);
    event $Sponsor(address indexed sponsor, bool yesOrNo);
    event $SelectSponsor(address indexed sponsor);
}


library proto {
    function Of(address addr) internal view returns(PrototypeInterface) {
        return Prototype(uint160(bytes9("Prototype"))).Of(addr);
    }
}