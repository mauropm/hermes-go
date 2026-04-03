package tool

import (
	"context"
	"fmt"
	"time"
)

type handlerTool struct {
	name        string
	description string
	schema      map[string]any
	handler     func(args map[string]any) string
	parallel    bool
}

func (h *handlerTool) Name() string           { return h.name }
func (h *handlerTool) Description() string    { return h.description }
func (h *handlerTool) Schema() map[string]any { return h.schema }
func (h *handlerTool) ParallelSafe() bool     { return h.parallel }
func (h *handlerTool) Execute(_ context.Context, input ToolInput) (ToolOutput, error) {
	result := h.handler(input.Arguments)
	return ToolOutput{
		Success: true,
		Data:    result,
	}, nil
}

func RegisterBuiltinTools(registry *Registry) error {
	if err := registry.Register(&handlerTool{
		name:        "get_time",
		description: "Get the current date and time in UTC",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		handler: func(args map[string]any) string {
			return toolResultJSON(true, map[string]string{
				"time": time.Now().UTC().Format(time.RFC3339),
				"unix": fmt.Sprintf("%d", time.Now().Unix()),
			}, "")
		},
		parallel: true,
	}); err != nil {
		return err
	}

	if err := registry.Register(&handlerTool{
		name:        "calculator",
		description: "Perform basic arithmetic calculations. Supports +, -, *, /, and parentheses.",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "Arithmetic expression (numbers and +, -, *, /, () only)",
				},
			},
			"required": []string{"expression"},
		},
		handler: func(args map[string]any) string {
			expr, ok := args["expression"].(string)
			if !ok {
				return toolResultJSON(false, nil, "expression argument is required and must be a string")
			}
			if len(expr) > 1000 {
				return toolResultJSON(false, nil, "expression too long")
			}
			for _, ch := range expr {
				if ch != ' ' && ch != '+' && ch != '-' && ch != '*' && ch != '/' &&
					ch != '(' && ch != ')' && ch != '.' && (ch < '0' || ch > '9') {
					return toolResultJSON(false, nil, "expression contains invalid characters; only numbers and +, -, *, /, () are allowed")
				}
			}
			result, err := safeEval(expr)
			if err != nil {
				return toolResultJSON(false, nil, fmt.Sprintf("calculation error: %v", err))
			}
			return toolResultJSON(true, map[string]any{
				"expression": expr,
				"result":     result,
			}, "")
		},
		parallel: true,
	}); err != nil {
		return err
	}

	if err := registry.Register(&handlerTool{
		name:        "help",
		description: "Get help about available tools and how to use this assistant",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		handler: func(args map[string]any) string {
			toolList := registry.Names()
			return toolResultJSON(true, map[string]any{
				"available_tools": toolList,
				"note":            "I am a secure AI assistant. I cannot execute commands, access files, or retrieve secrets.",
			}, "")
		},
		parallel: true,
	}); err != nil {
		return err
	}

	if err := RegisterWebSearchTool(registry); err != nil {
		return err
	}

	return nil
}

func safeEval(expr string) (float64, error) {
	if len(expr) > 1000 {
		expr = expr[:1000]
	}
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
