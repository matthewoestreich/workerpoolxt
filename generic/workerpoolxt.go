package generic

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
)

// New creates a new workerpoolxt
func New[T any](size int) *WorkerPoolXT[T] {
	return &WorkerPoolXT[T]{
		WorkerPool: workerpool.New(size),
		results:    []*Result[T]{},
	}
}

// WithWorkerPool creates a new workerpoolxt using an existing WorkerPool
func WithWorkerPool[T any](workerpool *workerpool.WorkerPool) *WorkerPoolXT[T] {
	return &WorkerPoolXT[T]{
		WorkerPool: workerpool,
		results:    []*Result[T]{},
	}
}

// WorkerPoolXT wraps workerpool
type WorkerPoolXT[T any] struct {
	*workerpool.WorkerPool
	resultsMutex sync.Mutex
	results      []*Result[T]
	once         sync.Once
}

// Results gets results, if any exist.
// You should call |.StopWait()| first.
// Preferrably you should use |allResult := .StopWaitXT()|
func (wp *WorkerPoolXT[T]) Results() []*Result[T] {
	return wp.results
}

// SubmitXT submits a Job to workerpool
func (wp *WorkerPoolXT[T]) SubmitXT(job *Job[T]) error {
	if job.Function == nil {
		return errors.New("job.Function is nil")
	}

	submitErr := wp.trySubmit(func() {
		defer recoverFromJobPanic(wp, job)

		job.startedAt = time.Now()
		data, err := job.Function()
		duration := time.Since(job.startedAt)

		wp.appendResult(&Result[T]{
			Name:     job.Name,
			Error:    err,
			Duration: duration,
			Data:     data,
		})
	})

	if submitErr != nil {
		wp.appendResult(&Result[T]{
			Name: job.Name,
			Error: PanicRecoveryError[T]{
				Job:     job,
				Message: fmt.Sprintf("failure during job submission: %v", submitErr),
			},
			Duration: 0,
			Data:     *new(T),
		})
	}

	return nil
}

// StopWaitXT blocks main thread and waits for all jobs
func (wp *WorkerPoolXT[T]) StopWaitXT() []*Result[T] {
	wp.once.Do(func() {
		wp.StopWait()
	})
	return wp.results
}

// Gives us an error if there is a panic during job submission.
func (wp *WorkerPoolXT[T]) trySubmit(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during submit: %v", r)
		}
	}()
	wp.Submit(fn)
	return nil
}

// Handles locking/unlocking mutex while appending result to results.
func (wp *WorkerPoolXT[T]) appendResult(result *Result[T]) {
	wp.resultsMutex.Lock()
	wp.results = append(wp.results, result)
	wp.resultsMutex.Unlock()
}

// Result is a Job resut
type Result[T any] struct {
	Error    error
	Data     T
	Name     string
	Duration time.Duration
}

func (r Result[T]) String() string {
	return fmt.Sprintf("Job: %s | Data: %v | Err: %v | Took: %s", r.Name, r.Data, r.Error, r.Duration)
}

// Job holds job related info.
// Job is what is passed to wpxt.SubmitXT
type Job[T any] struct {
	Name      string
	Function  func() (T, error)
	startedAt time.Time
}

// PanicRecoveryError is what gets thrown during job panic recovery
type PanicRecoveryError[T any] struct {
	Job     *Job[T]
	Message string
}

func (e PanicRecoveryError[T]) Error() string {
	return "[PanicRecoveryError] : " + e.Message
}

// Helper function to recover from a panic within a job
func recoverFromJobPanic[T any](wp *WorkerPoolXT[T], j *Job[T]) {
	if r := recover(); r != nil {
		wp.appendResult(&Result[T]{
			Name: j.Name,
			Error: PanicRecoveryError[T]{
				Message: fmt.Sprintf("Job recovered from panic \"%v\"", r),
				Job:     j,
			},
			Data:     *new(T),
			Duration: time.Since(j.startedAt),
		})
	}
}
