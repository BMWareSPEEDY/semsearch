// Example C file for testing

#include <stdio.h>
#include <stdlib.h>

struct Point {
    double x;
    double y;
};

struct Calculator {
    int precision;
};

struct Calculator* calculator_new(int precision) {
    struct Calculator* calc = malloc(sizeof(struct Calculator));
    calc->precision = precision;
    return calc;
}

int calculator_add(struct Calculator* calc, int a, int b) {
    return a + b;
}

int calculator_multiply(struct Calculator* calc, int a, int b) {
    return a * b;
}

void calculator_free(struct Calculator* calc) {
    free(calc);
}

enum Status {
    ACTIVE,
    INACTIVE,
    PENDING
};

void process_data(struct Point* points, int count) {
    for (int i = 0; i < count; i++) {
        printf("Point: (%.2f, %.2f)\n", points[i].x, points[i].y);
    }
}

int main() {
    struct Calculator* calc = calculator_new(2);
    printf("%d\n", calculator_add(calc, 1, 2));
    calculator_free(calc);
    return 0;
}