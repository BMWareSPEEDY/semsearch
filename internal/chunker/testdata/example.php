<?php
// Example PHP file for testing

namespace App\Calculator;

class Calculator
{
    private int $precision;

    public function __construct(int $precision = 2)
    {
        $this->precision = $precision;
    }

    public function add(int $a, int $b): int
    {
        return $a + $b;
    }

    public function multiply(int $a, int $b): int
    {
        return $a * $b;
    }
}

interface Shape
{
    public function area(): float;
}

class Circle implements Shape
{
    private float $radius;

    public function __construct(float $radius)
    {
        $this->radius = $radius;
    }

    public function area(): float
    {
        return M_PI * $this->radius * $this->radius;
    }
}

enum Status: string
{
    case Active = 'active';
    case Inactive = 'inactive';
    case Pending = 'pending';
}

function processData(array $points): void
{
    foreach ($points as $point) {
        echo "Point: ({$point['x']}, {$point['y']})\n";
    }
}