// Example C# file for testing

using System;
using System.Collections.Generic;

namespace Example
{
    public class Calculator
    {
        private int precision;

        public Calculator(int precision = 2)
        {
            this.precision = precision;
        }

        public int Add(int a, int b)
        {
            return a + b;
        }

        public int Multiply(int a, int b)
        {
            return a * b;
        }
    }

    public struct Point
    {
        public double X;
        public double Y;
    }

    public interface IShape
    {
        double Area();
    }

    public class Circle : IShape
    {
        private double radius;

        public Circle(double radius)
        {
            this.radius = radius;
        }

        public double Area()
        {
            return Math.PI * radius * radius;
        }
    }

    public enum Status
    {
        Active,
        Inactive,
        Pending
    }

    class Program
    {
        static void Main(string[] args)
        {
            var calc = new Calculator();
            Console.WriteLine(calc.Add(1, 2));
        }
    }
}