package tools

import (
	"fmt"
	"time"

	"github.com/nousresearch/hermes-go/llm"
	"github.com/nousresearch/hermes-go/security"
)

func RegisterBuiltinTools(registry *Registry) error {
	if err := registry.Register(&Tool{
		Name:        "get_time",
		Description: "Get the current date and time in UTC",
		Schema:      DefaultSchema("get_time", "Get current time", nil, nil),
		Handler: func(args map[string]interface{}) string {
			return llm.ToolResultJSON(true, map[string]string{
				"time": time.Now().UTC().Format(time.RFC3339),
				"unix": fmt.Sprintf("%d", time.Now().Unix()),
			}, "")
		},
		Parallel: true,
	}); err != nil {
		return err
	}

	if err := registry.Register(&Tool{
		Name:        "calculator",
		Description: "Perform basic arithmetic calculations. Supports +, -, *, /, and parentheses.",
		Schema: DefaultSchema("calculator", "Calculate expression", map[string]interface{}{
			"expression": map[string]interface{}{
				"type":        "string",
				"description": "Arithmetic expression (numbers and +, -, *, /, () only)",
			},
		}, []string{"expression"}),
		Handler: func(args map[string]interface{}) string {
			expr, ok := args["expression"].(string)
			if !ok {
				return llm.ToolResultJSON(false, nil, "expression argument is required and must be a string")
			}

			if len(expr) > 1000 {
				return llm.ToolResultJSON(false, nil, "expression too long")
			}

			for _, ch := range expr {
				if ch != ' ' && ch != '+' && ch != '-' && ch != '*' && ch != '/' &&
					ch != '(' && ch != ')' && ch != '.' && (ch < '0' || ch > '9') {
					return llm.ToolResultJSON(false, nil, "expression contains invalid characters; only numbers and +, -, *, /, () are allowed")
				}
			}

			result, err := safeEval(expr)
			if err != nil {
				return llm.ToolResultJSON(false, nil, fmt.Sprintf("calculation error: %v", err))
			}

			return llm.ToolResultJSON(true, map[string]interface{}{
				"expression": expr,
				"result":     result,
			}, "")
		},
		Parallel: true,
	}); err != nil {
		return err
	}

	if err := registry.Register(&Tool{
		Name:        "help",
		Description: "Get help about available tools and how to use this assistant",
		Schema:      DefaultSchema("help", "Get help", nil, nil),
		Handler: func(args map[string]interface{}) string {
			toolList := registry.ListTools()
			return llm.ToolResultJSON(true, map[string]interface{}{
				"available_tools": toolList,
				"note":            "I am a secure AI assistant. I cannot execute commands, access files, or retrieve secrets.",
			}, "")
		},
		Parallel: true,
	}); err != nil {
		return err
	}

	return nil
}

func safeEval(expr string) (float64, error) {
	expr = security.Truncate(expr, 1000)

	var pos int
	result, err := parseExpr(expr, &pos)
	if err != nil {
		return 0, err
	}

	if pos != len(expr) {
		return 0, fmt.Errorf("unexpected characters after expression")
	}

	return result, nil
}

func parseExpr(expr string, pos *int) (float64, error) {
	left, err := parseTerm(expr, pos)
	if err != nil {
		return 0, err
	}

	for *pos < len(expr) {
		skipSpaces(expr, pos)
		if *pos >= len(expr) {
			break
		}

		op := expr[*pos]
		if op != '+' && op != '-' {
			break
		}
		*pos++

		right, err := parseTerm(expr, pos)
		if err != nil {
			return 0, err
		}

		if op == '+' {
			left += right
		} else {
			left -= right
		}
	}

	return left, nil
}

func parseTerm(expr string, pos *int) (float64, error) {
	left, err := parseFactor(expr, pos)
	if err != nil {
		return 0, err
	}

	for *pos < len(expr) {
		skipSpaces(expr, pos)
		if *pos >= len(expr) {
			break
		}

		op := expr[*pos]
		if op != '*' && op != '/' {
			break
		}
		*pos++

		right, err := parseFactor(expr, pos)
		if err != nil {
			return 0, err
		}

		if op == '*' {
			left *= right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		}
	}

	return left, nil
}

func parseFactor(expr string, pos *int) (float64, error) {
	skipSpaces(expr, pos)

	if *pos >= len(expr) {
		return 0, fmt.Errorf("unexpected end of expression")
	}

	if expr[*pos] == '(' {
		*pos++
		result, err := parseExpr(expr, pos)
		if err != nil {
			return 0, err
		}
		skipSpaces(expr, pos)
		if *pos >= len(expr) || expr[*pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		*pos++
		return result, nil
	}

	return parseNumber(expr, pos)
}

func parseNumber(expr string, pos *int) (float64, error) {
	start := *pos
	hasDot := false

	for *pos < len(expr) {
		ch := expr[*pos]
		if ch >= '0' && ch <= '9' {
			*pos++
		} else if ch == '.' && !hasDot {
			hasDot = true
			*pos++
		} else {
			break
		}
	}

	if *pos == start {
		return 0, fmt.Errorf("expected number at position %d", start)
	}

	var result float64
	_, err := fmt.Sscanf(expr[start:*pos], "%f", &result)
	return result, err
}

func skipSpaces(expr string, pos *int) {
	for *pos < len(expr) && expr[*pos] == ' ' {
		*pos++
	}
}
