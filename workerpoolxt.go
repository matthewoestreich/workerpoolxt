package workerpoolxt

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
)

// New creates a new workerpoolxt
func New(size int) *workerpoolxt {
	return &workerpoolxt{
		WorkerPool: workerpool.New(size),
		results:    []*Result{},
	}
}

// WithWorkerPool creates a new workerpoolxt using an existing WorkerPool
func WithWorkerPool(workerpool *workerpool.WorkerPool) *workerpoolxt {
	return &workerpoolxt{
		WorkerPool: workerpool,
		results:    []*Result{},
	}
}

// WorkerPoolXT wraps workerpool
type WorkerPoolXT interface {
	SubmitXT(*Job) error
	StopWaitXT() []*Result
	Results() []*Result
}

// Concrete type backing WorkerPoolXT interface
type workerpoolxt struct {
	*workerpool.WorkerPool
	resultsMutex sync.Mutex
	results      []*Result
	once         sync.Once
}

// Results gets results, if any exist.
// You should call |.StopWait()| first.
// Preferrably you should use |allResult := .StopWaitXT()|
func (wp *workerpoolxt) Results() []*Result {
	return wp.results
}

// SubmitXT submits a Job to workerpool
func (wp *workerpoolxt) SubmitXT(job *Job) error {
	if job.Function == nil {
		return errors.New("job.Function is nil")
	}

	submitErr := wp.trySubmit(func() {
		defer recoverFromJobPanic(wp, job)

		job.startedAt = time.Now()
		data, err := job.Function()
		duration := time.Since(job.startedAt)

		wp.appendResult(&Result{
			Name:     job.Name,
			Error:    err,
			Duration: duration,
			Data:     data,
		})
	})

	if submitErr != nil {
		wp.appendResult(&Result{
			Name: job.Name,
			Error: PanicRecoveryError{
				Job:     job,
				Message: fmt.Sprintf("failure during job submission: %v", submitErr),
			},
			Duration: 0,
			Data:     nil,
		})
	}

	return nil
}

// StopWaitXT blocks main thread and waits for all jobs
func (wp *workerpoolxt) StopWaitXT() []*Result {
	wp.once.Do(func() {
		wp.StopWait()
	})
	return wp.results
}

// Gives us an error if there is a panic during job submission.
func (wp *workerpoolxt) trySubmit(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during submit: %v", r)
		}
	}()
	wp.Submit(fn)
	return nil
}

// Handles locking/unlocking mutex while appending result to results.
func (wp *workerpoolxt) appendResult(result *Result) {
	wp.resultsMutex.Lock()
	wp.results = append(wp.results, result)
	wp.resultsMutex.Unlock()
}

// Result is a Job resut
type Result struct {
	Error    error
	Data     any
	Name     string
	Duration time.Duration
}

func (r Result) String() string {
	return fmt.Sprintf("Job: %s | Data: %v | Err: %v | Took: %s", r.Name, r.Data, r.Error, r.Duration)
}

// Job holds job related info.
// Job is what is passed to wpxt.SubmitXT
type Job struct {
	Name      string
	Function  func() (any, error)
	startedAt time.Time
}

// PanicRecoveryError is what gets thrown during job panic recovery
type PanicRecoveryError struct {
	Job     *Job
	Message string
}

func (e PanicRecoveryError) Error() string {
	return "[PanicRecoveryError] : " + e.Message
}

// Helper function to recover from a panic within a job
func recoverFromJobPanic(wp *workerpoolxt, j *Job) {
	if r := recover(); r != nil {
		wp.appendResult(&Result{
			Name: j.Name,
			Error: PanicRecoveryError{
				Message: fmt.Sprintf("Job recovered from panic \"%v\"", r),
				Job:     j,
			},
			Data:     nil,
			Duration: time.Since(j.startedAt),
		})
	}
}
