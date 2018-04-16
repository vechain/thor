pragma solidity ^0.4.18;

contract Prototype {
}

contract PrototypeNative {
    function prototype_master() public view returns(address master);
    function prototype_setMaster(address newMaster) public;

    function prototype_energy() public view returns(uint256);
    function prototype_transferEnergy(uint256 amount) public returns(bool);
    function prototype_transferEnergyTo(address to, uint256 amount) public returns(bool);

    function prototype_isUser(address user) public view returns(bool);
    function prototype_userCredit(address user) public view returns(uint256 remainedCredit);
    function prototype_addUser(address user) public returns(bool added);
    function prototype_removeUser(address user) public returns(bool removed);

    function prototype_userPlan() public view returns(uint256 credit, uint256 recoveryRate);
    function prototype_setUserPlan(uint256 credit, uint256 recoveryRate) public;

    function prototype_sponsor(bool yesOrNo) public returns(bool);
    function prototype_isSponsor(address sponsor) public view returns(bool);
    function prototype_selectSponsor(address sponsor) public returns(bool selected);
    function prototype_currentSponsor() public view returns(address);

    event prototype_SetMaster(address indexed newMaster);
    event prototype_AddRemoveUser(address indexed user, bool addOrRemove);
    event prototype_SetUserPlan(uint256 credit, uint256 recoveryRate);
    event prototype_Sponsor(address indexed sponsor, bool yesOrNo);
    event prototype_SelectSponsor(address indexed sponsor);
}