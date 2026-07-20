package filtervalidate

import (
	"strings"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

func Validate(expr types.FilterExpr) error {
	return ValidateWithLimits(expr, Limits{})
}

type Limits struct {
	MaxDepth      int
	MaxNodes      int
	MaxListValues int
}

func ValidateWithLimits(expr types.FilterExpr, limits Limits) error {
	state := validationState{limits: limits}
	return state.validate(expr, 1)
}

func Fields(expr types.FilterExpr) []string {
	out := []string{}
	seen := map[string]struct{}{}
	var visit func(types.FilterExpr)
	visit = func(current types.FilterExpr) {
		switch value := current.(type) {
		case types.AndExpr:
			for _, term := range value.Terms {
				visit(term)
			}
		case types.OrExpr:
			for _, term := range value.Terms {
				visit(term)
			}
		case types.NotExpr:
			visit(value.Term)
		case types.TermExpr:
			appendField(&out, seen, value.Field)
		case types.RangeExpr:
			appendField(&out, seen, value.Field)
		case types.ExistsExpr:
			appendField(&out, seen, value.Field)
		}
	}
	visit(expr)
	return out
}

func appendField(out *[]string, seen map[string]struct{}, field string) {
	field = strings.TrimSpace(field)
	if field == "" {
		return
	}
	if _, ok := seen[field]; ok {
		return
	}
	seen[field] = struct{}{}
	*out = append(*out, field)
}

type validationState struct {
	limits Limits
	nodes  int
}

func (s *validationState) validate(expr types.FilterExpr, depth int) error {
	if expr == nil {
		return nil
	}
	s.nodes++
	if s.limits.MaxDepth > 0 && depth > s.limits.MaxDepth {
		return errs.InvalidFilter("filter depth exceeds configured limit", map[string]any{"maximum": s.limits.MaxDepth})
	}
	if s.limits.MaxNodes > 0 && s.nodes > s.limits.MaxNodes {
		return errs.InvalidFilter("filter node count exceeds configured limit", map[string]any{"maximum": s.limits.MaxNodes})
	}
	switch v := expr.(type) {
	case types.AndExpr:
		for _, term := range v.Terms {
			if err := s.validate(term, depth+1); err != nil {
				return err
			}
		}
	case types.OrExpr:
		for _, term := range v.Terms {
			if err := s.validate(term, depth+1); err != nil {
				return err
			}
		}
	case types.NotExpr:
		return s.validate(v.Term, depth+1)
	case types.TermExpr:
		if strings.TrimSpace(v.Field) == "" {
			return errs.InvalidFilter("filter field is required", nil)
		}
		switch v.Op {
		case types.FilterOpEQ, types.FilterOpNEQ, types.FilterOpIn, types.FilterOpContains:
		default:
			return errs.InvalidFilter("unsupported filter operator", map[string]any{"field": v.Field, "op": v.Op})
		}
		if v.Op == types.FilterOpIn {
			length := 0
			switch value := v.Value.(type) {
			case []string:
				length = len(value)
			case []any:
				length = len(value)
			default:
				return errs.InvalidFilter("filter in operator expects a list value", map[string]any{"field": v.Field, "value": v.Value})
			}
			if s.limits.MaxListValues > 0 && length > s.limits.MaxListValues {
				return errs.InvalidFilter("filter list exceeds configured limit", map[string]any{"field": v.Field, "actual": length, "maximum": s.limits.MaxListValues})
			}
		}
	case types.RangeExpr:
		if strings.TrimSpace(v.Field) == "" {
			return errs.InvalidFilter("range field is required", nil)
		}
	case types.ExistsExpr:
		if strings.TrimSpace(v.Field) == "" {
			return errs.InvalidFilter("exists field is required", nil)
		}
	default:
		return errs.InvalidFilter("unsupported filter expression", map[string]any{"type": expr})
	}
	return nil
}
