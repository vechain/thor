//SPDX-License-Identifier: MIT
pragma solidity 0.8.20;

contract TestContract {

    struct ComplexStruct {
        uint256 id;
        string name;
        address owner;
        uint256[] values;
    }

    function getComplexStruct() external pure returns (ComplexStruct memory) {
        uint256[] memory values = new uint256[](3);
        values[0] = 10;
        values[1] = 20;
        values[2] = 30;

        ComplexStruct memory cs = ComplexStruct({
            id: 1,
            name: "Test",
            owner: address(0x1234567890123456789012345678901234567890),
            values: values
        });

        return cs;
    }
}