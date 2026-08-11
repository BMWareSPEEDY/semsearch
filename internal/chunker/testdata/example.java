// Example Java file for testing

package com.example;

import java.util.List;
import java.util.ArrayList;

public class Calculator {
    private int precision;

    public Calculator(int precision) {
        this.precision = precision;
    }

    public int add(int a, int b) {
        return a + b;
    }

    public int multiply(int a, int b) {
        return a * b;
    }

    public static void main(String[] args) {
        Calculator calc = new Calculator(2);
        System.out.println(calc.add(1, 2));
    }
}

interface Shape {
    double area();
}

class Circle implements Shape {
    private double radius;

    public Circle(double radius) {
        this.radius = radius;
    }

    @Override
    public double area() {
        return Math.PI * radius * radius;
    }
}

enum Color {
    RED, GREEN, BLUE
}