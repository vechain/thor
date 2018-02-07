pragma solidity ^0.4.18;

/// @title Authority manages the whitelist of block proposers.
contract Authority {
   
    // address of voting contract, which controls the whitelist.
    address public voting;
    mapping(address => string) identities;

    // @notice initialize the contract.
    // @param _voting the account controls this contract.
    function _initialize(address _voting) public {
        require(msg.sender == address(this));
        voting = _voting;
    }

    // @notice authorize someone to be a block proposer.
    // It will be reverted if someone already listed, 
    // @param _addr address of someone.
    // @param _identity identity to identify someone. Must be non-empty. 
    function authorize(address _addr, string _identity) public {
        require(msg.sender == voting || msg.sender == address(this));
        require(_addr != 0 && bytes(_identity).length > 0);

        require(this.nativeAddProposer(_addr));
        identities[_addr] = _identity;

        Authorize(_addr, _identity);
    }

    // @notice deauthorize a block proposer by its address.
    // @param _addr address of the proposer.
    function deauthorize(address _addr) public {
        require(msg.sender == voting);

        require(this.nativeRemoveProposer(_addr));
        identities[_addr] = "";

        Deauthorize(_addr);
    }

    function statusOf(address _addr) public view returns(bool found, uint32 status) {
        return this.nativeGetProposer(_addr);
    }

    function identityOf(address _addr) public view returns(string) {
        return identities[_addr];
    }

    function nativeAddProposer(address addr) public returns(bool) {}
    function nativeRemoveProposer(address addr) public returns(bool) {}
    function nativeGetProposer(address addr) public view returns(bool, uint32) {}

    // fired when an address authorized to be a proposer.
    event Authorize(address indexed _addr, string _identity);
    // fired when an address deauthorized.
    event Deauthorize(address indexed _addr);
}

