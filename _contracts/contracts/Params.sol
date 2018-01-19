pragma solidity ^0.4.18;
import './Owned.sol';

contract Params {
    // address of voting contract, which controls the params.
    address public voting;

    mapping(string=>uint256) values;

    //balance growth rate at `timestamp`
    struct BalanceBirth {
        uint256 timestamp;  // `timestamp` changes if birth updated.
        uint256 birth;      // how many tokens grown by per vet per second
    }

    BalanceBirth[] birthRevisions;

    event SetBalanceBirth(address indexed executor,uint256 time,uint256 birth);

    ///@notice adjust balance growth rate to `_birth`
    ///@param _birth how much energy grows by per vet per second
    function setBalanceBirth(uint256 _birth) public {
        require(msg.sender == address(this));
        require(_birth > 0);
        
        uint256 latestVer = lenthOfRevisions()-1;
        uint256 latestime = timeWithVer(latestVer);
        if (now == latestime) {
            birthRevisions[latestVer].birth = _birth;
        } else {
            birthRevisions.push(BalanceBirth(now,_birth));
        }
        SetBalanceBirth(msg.sender,now,_birth);
    }

    function birthWithVer(uint256 version) public view returns(uint256) {
        require(version <= lenthOfRevisions()-1);
        if (birthRevisions.length == 0) {
            return 0;
        }
        return birthRevisions[version].birth;
    }

    function timeWithVer(uint256 version) public view returns(uint256) {
        require(version <= lenthOfRevisions()-1);
        if (birthRevisions.length == 0) {
            return 0;
        }
        return birthRevisions[version].timestamp;
    }

    function lenthOfRevisions() public view returns(uint256) {
        return birthRevisions.length;
    }

    function initialize(address _voting) public {
        require(msg.sender == address(this));
        voting = _voting;        
    }

    function preset(string _key, uint256 _value) public {
        require(msg.sender == address(this));
        values[_key] = _value;
    }

    function get(string _key) public view returns(uint256) {
        return values[_key];
    }

    function set(string _key, uint256 _value) public {
        require(msg.sender == voting);
        values[_key] = _value;
        
        Set(_key, _key, _value);
    }

    event Set(string indexed _indexedKey, string _key, uint256 _value);
}
