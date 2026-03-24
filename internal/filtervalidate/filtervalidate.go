package filtervalidate

import (
	"strings"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

func Validate(expr types.FilterExpr) error {
	if expr == nil {
		return nil
	}
	switch v := expr.(type) {
	case types.AndExpr:
		for _, term := range v.Terms {
			if err := Validate(term); err != nil {
				return err
			}
		}
	case types.OrExpr:
		for _, term := range v.Terms {
			if err := Validate(term); err != nil {
				return err
			}
		}
	case types.NotExpr:
		return Validate(v.Term)
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
			switch v.Value.(type) {
			case []string, []any:
			default:
				return errs.InvalidFilter("filter in operator expects a list value", map[string]any{"field": v.Field, "value": v.Value})
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
