pragma solidity ^0.4.18;
import './Owned.sol';
import './Constants.sol';

contract Authority is Owned {
    event Registered(address indexed _addr);
    event Authorised(address indexed _witness, bool _b);

    mapping(address => string) registry;

    address[] public witnesses;
    mapping(address => uint) witnessMap;

    address[] public absentee;
    mapping(address => uint) absenteeMap;

    function register(string _desc) public {
        require(bytes(_desc).length > 0);
        require(bytes(registry[msg.sender]).length == 0);

        registry[msg.sender] = _desc;        
        Registered(msg.sender);
    }
    
    function authorise (address _witness, bool _b) public onlyOwner {
        uint pos = witnessMap[_witness];
        if (_b) {            
            require(pos == 0);
            witnesses.push(_witness);
            witnessMap[_witness] = witnesses.length;
        } else {
            require(pos > 0);
            witnesses[pos - 1] = witnesses[witnesses.length - 1];
            witnesses.length -= 1;
            witnessMap[_witness] = 0;

            _absent(_witness, false);
        }
        Authorised(_witness, _b);
    }

    function _absent(address _witness, bool _b) internal returns (bool) {
        uint pos = absenteeMap[_witness];
        if (_b) {
            if (pos > 0)
                return false;
            absentee.push(_witness);
            absenteeMap[_witness] = absentee.length;
        } else {
            if (pos == 0)      
                return false;
            absentee[pos - 1] = absentee[absentee.length - 1];
            absentee.length -= 1;
            absenteeMap[_witness] = 0;
        }
        return true;
    }

    function absent(address _witness, bool _b) public {
        require(msg.sender == Constants.god());
        require(_absent(_witness, _b));
    }
}
