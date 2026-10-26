package qualityexpr

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

const MaxExpressionBytes = 2048

type Program struct {
	source string
	prog   *vm.Program
}

func Compile(expression string) (*Program, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, fmt.Errorf("expression is empty")
	}
	if len(expression) > MaxExpressionBytes {
		return nil, fmt.Errorf("expression exceeds %d bytes", MaxExpressionBytes)
	}
	prog, err := expr.Compile(expression,
		expr.Env(map[string]any{}),
		expr.AsBool(),
		expr.WarnOnAny(),
		expr.AllowUndefinedVariables(),
		expr.DisableAllBuiltins(),
		expr.EnableBuiltin("any"),
		expr.EnableBuiltin("all"),
		expr.EnableBuiltin("none"),
		expr.EnableBuiltin("filter"),
		expr.EnableBuiltin("len"),
		expr.EnableBuiltin("count"),
		expr.EnableBuiltin("int"),
		expr.EnableBuiltin("float"),
		expr.Function("notEmpty", func(params ...any) (any, error) {
			if len(params) != 1 {
				return false, fmt.Errorf("notEmpty expects 1 argument")
			}
			return valueNotEmpty(params[0]), nil
		}, new(func(any) bool)),
	)
	if err != nil {
		return nil, err
	}
	return &Program{source: expression, prog: prog}, nil
}

func (p *Program) Run(env map[string]any) (bool, error) {
	if p == nil || p.prog == nil {
		return false, fmt.Errorf("expression is not compiled")
	}
	if env == nil {
		env = map[string]any{}
	}
	output, err := expr.Run(p.prog, env)
	if err != nil {
		return false, err
	}
	passed, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("expression result is %T", output)
	}
	return passed, nil
}

func valueNotEmpty(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return reflected.Len() > 0
	case reflect.Pointer, reflect.Interface:
		if reflected.IsNil() {
			return false
		}
		return valueNotEmpty(reflected.Elem().Interface())
	default:
		return true
	}
}

func (p *Program) Source() string {
	if p == nil {
		return ""
	}
	return p.source
}
