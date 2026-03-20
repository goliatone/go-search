package types

type FilterExpr interface {
	isFilterExpr()
}

type AndExpr struct {
	Terms []FilterExpr `json:"terms"`
}
type OrExpr struct {
	Terms []FilterExpr `json:"terms"`
}
type NotExpr struct {
	Term FilterExpr `json:"term"`
}
type TermExpr struct {
	Field string   `json:"field"`
	Op    FilterOp `json:"op"`
	Value any      `json:"value"`
}
type RangeExpr struct {
	Field string `json:"field"`
	GTE   any    `json:"gte"`
	LTE   any    `json:"lte"`
}
type ExistsExpr struct {
	Field  string `json:"field"`
	Exists bool   `json:"exists"`
}

func (AndExpr) isFilterExpr()    {}
func (OrExpr) isFilterExpr()     {}
func (NotExpr) isFilterExpr()    {}
func (TermExpr) isFilterExpr()   {}
func (RangeExpr) isFilterExpr()  {}
func (ExistsExpr) isFilterExpr() {}

type FilterOp string

const (
	FilterOpEQ       FilterOp = "eq"
	FilterOpNEQ      FilterOp = "neq"
	FilterOpIn       FilterOp = "in"
	FilterOpContains FilterOp = "contains"
)

type SortDirection string

const (
	SortAsc  SortDirection = "asc"
	SortDesc SortDirection = "desc"
)

type Sort struct {
	Field     string        `json:"field"`
	Direction SortDirection `json:"direction"`
}

type FacetRequest struct {
	Field string `json:"field"`
	Limit int    `json:"limit"`
}
