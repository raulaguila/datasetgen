package output

import "time"

// Clock abstracts time for tests.
type Clock interface {
	Now() time.Time
}

// RealClock uses time.Now.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }
