pragma solidity ^0.4.11;

contract Calc {
    int public left;
    int public right;
    
    function Left(int num) {
        left = num;
    }
    
    function Right(int num) {
        right = num;
    }
    
    function Add() returns (int) {
        return left + right;
    }
}

contract AddTest {
    Calc public calc;
    
    function AddTest() {
        calc = new Calc();
    }
    
    function Add(int a, int b) returns (int) {
        calc.Left(a);
        calc.Right(b);
        return calc.Add();
    }
}