package builtin

import (
	"testing"
)

func TestCalculator_BasicOps(t *testing.T) {
	c := NewCalculatorTool()
	tests := []struct {
		expr     string
		expected string
	}{
		{"2 + 3", "5"},
		{"10 - 7", "3"},
		{"4 * 5", "20"},
		{"15 / 3", "5"},
		{"10 % 3", "1"},
		{"2 ^ 3", "8"},
		{"(2 + 3) * 4", "20"},
		{"-5 + 3", "-2"},
		{"2 * (3 + 4) - 5", "9"},
		{"10 / 3", "3.3333333333333335"},
	}

	for _, tt := range tests {
		result, err := c.evaluate(tt.expr)
		if err != nil {
			t.Errorf("evaluate(%q) error: %v", tt.expr, err)
			continue
		}
		t.Logf("%s = %v (expected %s)", tt.expr, result, tt.expected)
	}
}

func TestCalculator_Funcs(t *testing.T) {
	c := NewCalculatorTool()
	tests := []string{
		"sqrt(16)",
		"abs(-5)",
		"round(3.7)",
		"ceil(3.2)",
		"floor(3.8)",
		"log(1)",
		"exp(0)",
		"sin(0)",
		"cos(0)",
		"pow(2, 10)",
	}

	for _, expr := range tests {
		result, err := c.evaluate(expr)
		if err != nil {
			t.Errorf("evaluate(%q) error: %v", expr, err)
		} else {
			t.Logf("%s = %v", expr, result)
		}
	}
}

func TestCalculator_Errors(t *testing.T) {
	c := NewCalculatorTool()
	tests := []string{
		"10 / 0",
		"sqrt(-1)",
		"",
		"1 + ",
		"(1 + 2",
	}

	for _, expr := range tests {
		_, err := c.evaluate(expr)
		if err == nil {
			t.Errorf("evaluate(%q) expected error, got nil", expr)
		} else {
			t.Logf("evaluate(%q) correctly errored: %v", expr, err)
		}
	}
}
