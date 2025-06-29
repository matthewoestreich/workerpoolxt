package generic

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
)

// New creates a new workerpoolxt
func New[T any](size int) *WorkerpoolXT[T] {
	return &WorkerpoolXT[T]{
		WorkerPool: workerpool.New(size),
		results:    []Result[T]{},
	}
}

// WorkerpoolXT wraps workerpool
type WorkerpoolXT[T any] struct {
	*workerpool.WorkerPool
	resultsMutex sync.Mutex
	results      []Result[T]
	once         sync.Once
}

// Results gets results, if any exist.
// You should call |.StopWait()| first.
// Preferrably you should use |allResult := .StopWaitXT()|
func (wp *WorkerpoolXT[T]) Results() []Result[T] {
	return wp.results
}

// SubmitXT submits a Job to workerpool
func (wp *WorkerpoolXT[T]) SubmitXT(job Job[T]) (err error) {
	if job.Function == nil {
		return errors.New("job.Function is nil")
	}

	// If you try to submit a job on already closed channel.
	// eg. after calling |.StopWait()| or |.StopWaitXT()|
	defer recoverFromSubmitXTPanic(wp, job)

	wp.Submit(func() {
		resultChan := make(chan Result[T], 1)

		go func() {
			defer recoverFromJobPanic(job, resultChan)
			job.startedAt = time.Now()
			data, error := job.Function()
			resultChan <- Result[T]{
				Name:     job.Name,
				Error:    error,
				Duration: time.Since(job.startedAt),
				Data:     data,
			}
		}()

		res := <-resultChan
		wp.resultsMutex.Lock()
		wp.results = append(wp.results, res)
		wp.resultsMutex.Unlock()
	})

	return err
}

// StopWaitXT blocks main thread and waits for all jobs
func (wp *WorkerpoolXT[T]) StopWaitXT() []Result[T] {
	wp.once.Do(func() {
		wp.StopWait()
	})
	return wp.results
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
	Job     Job[T]
	Message string
}

func (e PanicRecoveryError[T]) Error() string {
	return "[PanicRecoveryError] : " + e.Message
}

// Helper function to recover from a panic within a job
func recoverFromJobPanic[T any](j Job[T], rc chan<- Result[T]) {
	if r := recover(); r != nil {
		rc <- Result[T]{
			Error:    PanicRecoveryError[T]{Message: fmt.Sprintf("Job \"%s\" recovered from panic \"%v\"", j.Name, r)},
			Name:     j.Name,
			Duration: time.Since(j.startedAt),
		}
	}
}

func recoverFromSubmitXTPanic[T any](xt *WorkerpoolXT[T], job Job[T]) {
	if r := recover(); r != nil {
		xt.resultsMutex.Lock()
		defer xt.resultsMutex.Unlock()

		xt.results = append(xt.results, Result[T]{
			Name: job.Name,
			Error: PanicRecoveryError[T]{
				Job:     job,
				Message: fmt.Sprintf("workerpool.Submit panicked: %v", r),
			},
			Duration: 0, // could be 0 since job never ran
			Data:     *new(T),
			// ^^^^^^^ WHY CAN'T I USE nil HERE?
		})
	}
}
