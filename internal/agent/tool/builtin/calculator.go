package builtin

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// CalculatorTool 安全地计算数学表达式。
type CalculatorTool struct{}

func NewCalculatorTool() *CalculatorTool {
	return &CalculatorTool{}
}

func (c *CalculatorTool) Name() string {
	return "calculator"
}

func (c *CalculatorTool) Description() string {
	return "安全计算数学表达式，支持 + - * / % ^ ( ) 及 sqrt/abs/round/ceil/floor/log/exp/sin/cos/tan 函数"
}

func (c *CalculatorTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{
				"type":        "string",
				"description": "数学表达式，如 (3 + 5) * 2 或 sqrt(16) + abs(-3)",
			},
		},
		"required": []string{"expression"},
	}
}

func (c *CalculatorTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	expr, ok := args["expression"].(string)
	if !ok || strings.TrimSpace(expr) == "" {
		return "", fmt.Errorf("缺少 expression 参数")
	}

	result, err := c.evaluate(expr)
	if err != nil {
		return "", err
	}

	if result == math.Trunc(result) {
		return fmt.Sprintf("%.0f", result), nil
	}
	return strconv.FormatFloat(result, 'f', -1, 64), nil
}

func (c *CalculatorTool) evaluate(expr string) (float64, error) {
	l := &lexer{input: []rune(expr)}
	result, err := parseExpr(l)
	if err != nil {
		return 0, err
	}
	if l.peek().kind != tokEOF {
		return 0, fmt.Errorf("表达式末尾有意外字符")
	}
	return result, nil
}

// --- Lexer ---

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNumber
	tokPlus
	tokMinus
	tokStar
	tokSlash
	tokPercent
	tokCaret
	tokLParen
	tokRParen
	tokFunc
	tokComma
)

type token struct {
	kind  tokenKind
	value string
}

type lexer struct {
	input []rune
	pos   int
	buf   *token
}

func (l *lexer) peek() token {
	if l.buf == nil {
		t := l.readNext()
		l.buf = &t
	}
	return *l.buf
}

func (l *lexer) next() token {
	if l.buf != nil {
		t := *l.buf
		l.buf = nil
		return t
	}
	return l.readNext()
}

func (l *lexer) readNext() token {
	l.skipSpace()
	if l.pos >= len(l.input) {
		return token{kind: tokEOF}
	}

	ch := l.input[l.pos]

	switch {
	case ch >= '0' && ch <= '9' || ch == '.':
		return l.readNumber()
	case ch == '+':
		l.pos++
		return token{kind: tokPlus, value: "+"}
	case ch == '-':
		l.pos++
		return token{kind: tokMinus, value: "-"}
	case ch == '*':
		l.pos++
		return token{kind: tokStar, value: "*"}
	case ch == '/':
		l.pos++
		return token{kind: tokSlash, value: "/"}
	case ch == '%':
		l.pos++
		return token{kind: tokPercent, value: "%"}
	case ch == '^':
		l.pos++
		return token{kind: tokCaret, value: "^"}
	case ch == '(':
		l.pos++
		return token{kind: tokLParen, value: "("}
	case ch == ')':
		l.pos++
		return token{kind: tokRParen, value: ")"}
	case ch == ',':
		l.pos++
		return token{kind: tokComma, value: ","}
	case unicode.IsLetter(ch):
		return l.readIdent()
	default:
		l.pos++
		return token{kind: tokEOF}
	}
}

func (l *lexer) readNumber() token {
	start := l.pos
	hasDot := false
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch >= '0' && ch <= '9' {
			l.pos++
		} else if ch == '.' && !hasDot {
			hasDot = true
			l.pos++
		} else {
			break
		}
	}
	return token{kind: tokNumber, value: string(l.input[start:l.pos])}
}

func (l *lexer) readIdent() token {
	start := l.pos
	for l.pos < len(l.input) && unicode.IsLetter(l.input[l.pos]) {
		l.pos++
	}
	return token{kind: tokFunc, value: string(l.input[start:l.pos])}
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.input) && unicode.IsSpace(l.input[l.pos]) {
		l.pos++
	}
}

// --- Recursive descent parser ---

func parseExpr(l *lexer) (float64, error) {
	left, err := parseTerm(l)
	if err != nil {
		return 0, err
	}
	for {
		switch l.peek().kind {
		case tokPlus:
			l.next()
			right, err := parseTerm(l)
			if err != nil {
				return 0, err
			}
			left += right
		case tokMinus:
			l.next()
			right, err := parseTerm(l)
			if err != nil {
				return 0, err
			}
			left -= right
		default:
			return left, nil
		}
	}
}

func parseTerm(l *lexer) (float64, error) {
	left, err := parseFactor(l)
	if err != nil {
		return 0, err
	}
	for {
		switch l.peek().kind {
		case tokStar:
			l.next()
			right, err := parseFactor(l)
			if err != nil {
				return 0, err
			}
			left *= right
		case tokSlash:
			l.next()
			right, err := parseFactor(l)
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("除零错误")
			}
			left /= right
		case tokPercent:
			l.next()
			right, err := parseFactor(l)
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("取模除零错误")
			}
			left = math.Mod(left, right)
		default:
			return left, nil
		}
	}
}

func parseFactor(l *lexer) (float64, error) {
	left, err := parseUnary(l)
	if err != nil {
		return 0, err
	}
	if l.peek().kind == tokCaret {
		l.next()
		right, err := parseFactor(l) // right-associative
		if err != nil {
			return 0, err
		}
		return math.Pow(left, right), nil
	}
	return left, nil
}

func parseUnary(l *lexer) (float64, error) {
	if l.peek().kind == tokMinus {
		l.next()
		val, err := parseUnary(l)
		if err != nil {
			return 0, err
		}
		return -val, nil
	}
	if l.peek().kind == tokPlus {
		l.next()
		return parseUnary(l)
	}
	return parsePrimary(l)
}

func parsePrimary(l *lexer) (float64, error) {
	tok := l.peek()

	switch tok.kind {
	case tokNumber:
		l.next()
		return strconv.ParseFloat(tok.value, 64)
	case tokLParen:
		l.next()
		val, err := parseExpr(l)
		if err != nil {
			return 0, err
		}
		if l.next().kind != tokRParen {
			return 0, fmt.Errorf("缺少右括号")
		}
		return val, nil
	case tokFunc:
		return parseFuncCall(l)
	default:
		return 0, fmt.Errorf("意外的字符: %s", tok.value)
	}
}

func parseFuncCall(l *lexer) (float64, error) {
	name := strings.ToLower(l.next().value)
	if l.next().kind != tokLParen {
		return 0, fmt.Errorf("%s 后需要 (", name)
	}

	// collect arguments
	var args []float64
	for {
		if l.peek().kind == tokRParen {
			break
		}
		val, err := parseExpr(l)
		if err != nil {
			return 0, err
		}
		args = append(args, val)
		if l.peek().kind == tokComma {
			l.next()
		}
	}
	l.next() // skip ')'

	return dispatchFunc(name, args)
}

func dispatchFunc(name string, args []float64) (float64, error) {
	if len(args) == 0 {
		return 0, fmt.Errorf("%s 至少需要一个参数", name)
	}

	// two-argument functions
	if len(args) == 2 {
		switch name {
		case "pow", "power":
			return math.Pow(args[0], args[1]), nil
		case "log":
			return math.Log(args[0]) / math.Log(args[1]), nil // log base
		}
	}

	if len(args) != 1 {
		return 0, fmt.Errorf("%s 需要 1 个参数，传入了 %d 个", name, len(args))
	}

	x := args[0]
	switch name {
	case "sqrt":
		if x < 0 {
			return 0, fmt.Errorf("sqrt 参数不能为负数")
		}
		return math.Sqrt(x), nil
	case "abs":
		return math.Abs(x), nil
	case "round":
		return math.Round(x), nil
	case "ceil":
		return math.Ceil(x), nil
	case "floor":
		return math.Floor(x), nil
	case "log", "ln":
		if x <= 0 {
			return 0, fmt.Errorf("log 参数必须大于 0")
		}
		return math.Log(x), nil
	case "log10":
		if x <= 0 {
			return 0, fmt.Errorf("log10 参数必须大于 0")
		}
		return math.Log10(x), nil
	case "exp":
		return math.Exp(x), nil
	case "sin":
		return math.Sin(x), nil
	case "cos":
		return math.Cos(x), nil
	case "tan":
		return math.Tan(x), nil
	case "asin":
		if x < -1 || x > 1 {
			return 0, fmt.Errorf("asin 参数必须在 [-1, 1] 范围内")
		}
		return math.Asin(x), nil
	case "acos":
		if x < -1 || x > 1 {
			return 0, fmt.Errorf("acos 参数必须在 [-1, 1] 范围内")
		}
		return math.Acos(x), nil
	case "atan":
		return math.Atan(x), nil
	default:
		return 0, fmt.Errorf("未知函数: %s", name)
	}
}
