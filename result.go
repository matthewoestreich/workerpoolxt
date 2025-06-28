package workerpoolxt

import (
	"time"
)

// Result is a resut
type Result struct {
	Error    error
	Data     any
	name     string
	duration time.Duration
}

// Duration does a thing
func (r *Result) Duration() time.Duration { return r.duration }

// Name does a thing
func (r *Result) Name() string { return r.name }