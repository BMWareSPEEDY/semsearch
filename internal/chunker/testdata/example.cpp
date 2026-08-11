// Example C++ file for testing

#include <iostream>
#include <vector>
#include <string>

class Calculator {
private:
    int precision;

public:
    Calculator(int p = 2) : precision(p) {}

    int add(int a, int b) {
        return a + b;
    }

    int multiply(int a, int b) {
        return a * b;
    }
};

struct Point {
    double x;
    double y;
};

enum class Status {
    Active,
    Inactive,
    Pending
};

void processData(const std::vector<Point>& points) {
    for (const auto& p : points) {
        std::cout << "Point: (" << p.x << ", " << p.y << ")" << std::endl;
    }
}

int main() {
    Calculator calc;
    std::cout << calc.add(1, 2) << std::endl;
    return 0;
}