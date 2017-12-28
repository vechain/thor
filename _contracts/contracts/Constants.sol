pragma solidity ^0.4.18;

library Constants {
    function params() pure internal returns (address) {
        return address(uint24(bytes3("par")));
    }

    function energy() pure internal returns (address) {
        return address(uint24(bytes3("eng")));
    }

    function authority() pure internal returns (address) {
        return address(uint24(bytes3("aut")));
    }

    function god() pure internal returns (address) {        
        return address(uint24(bytes3("god")));
    }
}
