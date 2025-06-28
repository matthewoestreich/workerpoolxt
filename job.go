package workerpoolxt

import (
	"time"
)

// Job holds job related info.
// Job is what is passed to wpxt.SubmitXT
type Job struct {
	Name      string
	Function  func() Result
	startedAt time.Time
}