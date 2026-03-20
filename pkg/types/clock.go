package types

import "time"

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

func SystemClock() Clock {
	return systemClock{}
}

type FixedClock struct {
	Time time.Time
}

func (c FixedClock) Now() time.Time {
	return c.Time
}
