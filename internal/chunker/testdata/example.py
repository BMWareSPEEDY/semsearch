"""Example Python module for testing."""

import os
import sys


class Calculator:
    """A simple calculator class."""

    def __init__(self, precision=2):
        self.precision = precision

    def add(self, a, b):
        """Add two numbers."""
        return round(a + b, self.precision)

    def multiply(self, a, b):
        """Multiply two numbers."""
        return round(a * b, self.precision)


def main():
    """Main entry point."""
    calc = Calculator()
    print(calc.add(1, 2))
    print(calc.multiply(3, 4))


if __name__ == "__main__":
    main()
