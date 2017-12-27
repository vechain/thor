pragma solidity ^0.4.18;
import './Owned.sol';
import './Constants.sol';

contract Authority is Owned {
    event Registered(address indexed _addr);
    event Authorised(address indexed _proposer, bool _b);

    mapping(address => string) registry;

    address[] public proposers;
    mapping(address => uint) proposerMap;

    address[] public absentee;
    mapping(address => uint) absenteeMap;

    function init() public {
        require(msg.sender == Constants.god());        
    }

    function register(string _desc) public {
        require(bytes(_desc).length > 0);
        require(bytes(registry[msg.sender]).length == 0);

        registry[msg.sender] = _desc;        
        Registered(msg.sender);
    }
    
    function authorise (address _proposer, bool _b) public onlyOwner {
        uint pos = proposerMap[_proposer];
        if (_b) {            
            require(pos == 0);
            proposers.push(_proposer);
            proposerMap[_proposer] = proposers.length;
        } else {
            require(pos > 0);
            proposers[pos - 1] = proposers[proposers.length - 1];
            proposers.length -= 1;
            proposerMap[_proposer] = 0;

            _absent(_proposer, false);
        }
        Authorised(_proposer, _b);
    }

    function _absent(address _proposer, bool _b) internal returns (bool) {
        if (proposerMap[_proposer] == 0) {
            return false;
        }
        uint pos = absenteeMap[_proposer];
        if (_b) {
            if (pos == 0) {
                absentee.push(_proposer);
                absenteeMap[_proposer] = absentee.length;
            }
        } else {
            if (pos > 0) { 
                absentee[pos - 1] = absentee[absentee.length - 1];
                absentee.length -= 1;
                absenteeMap[_proposer] = 0;
            }
        }
        return true;
    }

    function absent(address _proposer, bool _b) public {
        require(msg.sender == Constants.god());
        require(_absent(_proposer, _b));
    }
}
