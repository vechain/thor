pragma solidity ^0.4.18;

library Constants {
    function params() pure internal returns (address) {
        return address(uint24(bytes3("par")));
    }

    function authority() pure internal returns (address) {
        return address(uint24(bytes3("aut")));
    }

    function energy() pure internal returns (address) {
        return address(uint24(bytes3("eng")));
    }

    function voting() pure internal returns (address) {
        return address(uint24(bytes3("vot")));
    }
}
