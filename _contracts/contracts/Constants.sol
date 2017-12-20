pragma solidity ^0.4.18;

library Constants {
    function params() pure internal returns (address) {
        return address(bytes2("pa"));
    }

    function energy() pure internal returns (address) {
        return address(bytes2("en"));
    }

    function authority() pure internal returns (address) {
        return address(bytes2("au"));
    }

    function god() pure internal returns (address) {        
        return address(bytes2("go"));
    }
}
