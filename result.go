package workerpoolxt

import (
	"time"
)

// Result is a Job resut
type Result struct {
	Error    error
	Data     any
	name     string
	duration time.Duration
}

// Duration tells you how long a Job ran for
func (r *Result) Duration() time.Duration { return r.duration }

// Name is the Job name
func (r *Result) Name() string { return r.name }
