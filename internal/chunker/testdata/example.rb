# Example Ruby file for testing

module Calculator
  class Base
    attr_reader :precision

    def initialize(precision = 2)
      @precision = precision
    end

    def add(a, b)
      a + b
    end

    def multiply(a, b)
      a * b
    end
  end

  class Advanced < Base
    def divide(a, b)
      raise "Division by zero" if b.zero?
      a / b.to_f
    end
  end
end

class Point
  attr_accessor :x, :y

  def initialize(x, y)
    @x = x
    @y = y
  end

  def to_s
    "(#{@x}, #{@y})"
  end
end

enum = [:active, :inactive, :pending]

def process_data(points)
  points.each { |p| puts "Point: #{p}" }
end