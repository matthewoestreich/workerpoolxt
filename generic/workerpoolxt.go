package generic

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gammazero/workerpool"
)

// New creates a new workerpoolxt
func New[T any](size int) *WorkerPoolXT[T] {
	return WithWorkerPool[T](workerpool.New(size))
}

// WithWorkerPool creates a new workerpoolxt using an existing WorkerPool
func WithWorkerPool[T any](workerpool *workerpool.WorkerPool) *WorkerPoolXT[T] {
	wp := &WorkerPoolXT[T]{
		WorkerPool:       workerpool,
		processJobsQueue: make(chan *Job[T]),
		results:          []*Result[T]{},
		resultsWaitGroup: sync.WaitGroup{},
	}
	wp.resultsWaitGroup.Add(1)
	go wp.processResults()
	return wp
}

// WorkerPoolXT wraps workerpool
type WorkerPoolXT[T any] struct {
	*workerpool.WorkerPool
	stopped          atomic.Bool
	resultsMutex     sync.Mutex
	results          []*Result[T]
	processJobsQueue chan *Job[T]
	resultsWaitGroup sync.WaitGroup
	once             sync.Once
}

// Results gets results, if any exist.
// You should call |.StopWait()| first.
// Preferrably you should use |allResult := .StopWaitXT()|
func (wp *WorkerPoolXT[T]) Results() []*Result[T] {
	return wp.results
}

// SubmitXT submits a Job to workerpool
func (wp *WorkerPoolXT[T]) SubmitXT(job *Job[T]) error {
	if wp.stopped.Load() {
		return errors.New("pool is stopped! cannot reuse a stopped pool")
	}
	if job.Function == nil {
		return errors.New("job.Function is nil")
	}

	wp.resultsWaitGroup.Add(1)

	submitErr := wp.trySubmit(func() {
		defer recoverFromJobPanic(wp, job)
		job.startedAt = time.Now()
		job.data, job.error = job.Function()
		job.duration = time.Since(job.startedAt)
		wp.processJobsQueue <- job
		wp.resultsWaitGroup.Done()
	})

	if submitErr != nil {
		job.duration = 0
		job.data = *new(T)
		job.error = PanicRecoveryError[T]{
			Job:     job,
			Message: fmt.Sprintf("failure during job submission: %v", submitErr),
		}
		wp.processJobsQueue <- job
		wp.resultsWaitGroup.Done()
	}

	return nil
}

// StopWaitXT blocks main thread and waits for all jobs
func (wp *WorkerPoolXT[T]) StopWaitXT() []*Result[T] {
	if wp.stopped.Load() {
		return wp.Results()
	}
	wp.once.Do(func() {
		wp.StopWait()
		wp.stopped.Store(true)
		close(wp.processJobsQueue)
		wp.resultsWaitGroup.Wait()
	})
	return wp.Results()
}

func (p *WorkerPoolXT[T]) processResults() {
	for job := range p.processJobsQueue {
		p.appendResult(&Result[T]{
			Name:     job.Name,
			Data:     job.data,
			Duration: job.duration,
			Error:    job.error,
		})
	}
	p.resultsWaitGroup.Done()
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
	data      T
	error     error
	duration  time.Duration
	startedAt time.Time
}

func (j *Job[T]) String() string {
	return fmt.Sprintf("Job: %s | Data: %v | Err: %v | Took: %s", j.Name, j.data, j.error, j.duration)
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
		j.duration = time.Since(j.startedAt)
		j.data = *new(T)
		j.error = PanicRecoveryError[T]{
			Message: fmt.Sprintf("Job recovered from panic \"%v\"", r),
			Job:     j,
		}
		if !wp.stopped.Load() {
			wp.processJobsQueue <- j
		}
		wp.resultsWaitGroup.Done()
	}
}
