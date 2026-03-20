package types

type FilterExpr interface {
	isFilterExpr()
}

type AndExpr struct{ Terms []FilterExpr }
type OrExpr struct{ Terms []FilterExpr }
type NotExpr struct{ Term FilterExpr }
type TermExpr struct {
	Field string
	Op    FilterOp
	Value any
}
type RangeExpr struct {
	Field string
	GTE   any
	LTE   any
}
type ExistsExpr struct {
	Field  string
	Exists bool
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
	Field     string
	Direction SortDirection
}

type FacetRequest struct {
	Field string
	Limit int
}
