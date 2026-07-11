package ranking

import (
	"context"
	"testing"
)

func TestEvaluateSignalsBoundsPureFunctions(t *testing.T) {
	r := NewSignalRegistry()
	if err := r.Register("quality", SignalFunc(func(context.Context, SignalInput) (SignalResult, error) {
		return SignalResult{Value: 10, Reason: "fixture"}, nil
	})); err != nil {
		t.Fatal(err)
	}
	r.Seal()
	total, out, err := EvaluateSignals(context.Background(), r, []SignalSpec{{ID: "quality", Weight: 1, MinContribution: -.2, MaxContribution: .2}}, SignalInput{})
	if err != nil {
		t.Fatal(err)
	}
	if total != .2 || len(out) != 1 || out[0].Contribution != .2 {
		t.Fatalf("total=%v out=%#v", total, out)
	}
}
