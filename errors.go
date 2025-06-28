package workerpoolxt

// JobPanicError is what gets thrown during job panic recovery
type JobPanicError struct {
    Job Job
    Message string
}

func (e JobPanicError) Error() string {
    return "JobPanicError \"" + e.Job.Name + "\" " + e.Message
}