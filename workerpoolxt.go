package workerpoolxt

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gammazero/workerpool"
)

// New creates a new workerpoolxt
func New(size int) *WorkerPoolXT {
	return WithWorkerPool(workerpool.New(size))
}

// WithWorkerPool creates a new workerpoolxt using an existing WorkerPool
func WithWorkerPool(workerpool *workerpool.WorkerPool) *WorkerPoolXT {
	wp := &WorkerPoolXT{
		WorkerPool:       workerpool,
		processJobsQueue: make(chan *Job),
		results:          []*Result{},
		resultsWaitGroup: sync.WaitGroup{},
	}
	wp.resultsWaitGroup.Add(1)
	go wp.processResults()
	return wp
}

// WorkerPoolXT wraps workerpool
type WorkerPoolXT struct {
	*workerpool.WorkerPool
	stopped          atomic.Bool
	resultsMutex     sync.Mutex
	results          []*Result
	processJobsQueue chan *Job
	resultsWaitGroup sync.WaitGroup
	once             sync.Once
}

// Results gets results, if any exist.
// You should call |.StopWait()| first.
// Preferrably you should use |allResult := .StopWaitXT()|
func (wp *WorkerPoolXT) Results() []*Result {
	return wp.results
}

// SubmitXT submits a Job to workerpool
func (wp *WorkerPoolXT) SubmitXT(job *Job) error {
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
		job.data = nil
		job.error = PanicRecoveryError{
			Job:     job,
			Message: fmt.Sprintf("failure during job submission: %v", submitErr),
		}
		wp.processJobsQueue <- job
		wp.resultsWaitGroup.Done()
	}

	return nil
}

// StopWaitXT blocks main thread and waits for all jobs
func (wp *WorkerPoolXT) StopWaitXT() []*Result {
	if wp.stopped.Load() {
		return wp.Results()
	}
	wp.stopped.Store(true)
	wp.once.Do(func() {
		wp.StopWait()
		close(wp.processJobsQueue)
		wp.resultsWaitGroup.Wait()
	})
	return wp.Results()
}

func (p *WorkerPoolXT) processResults() {
	for job := range p.processJobsQueue {
		p.appendResult(&Result{
			Name:     job.Name,
			Data:     job.data,
			Duration: job.duration,
			Error:    job.error,
		})
	}
	p.resultsWaitGroup.Done()
}

// Gives us an error if there is a panic during job submission.
func (wp *WorkerPoolXT) trySubmit(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during submit: %v", r)
		}
	}()
	wp.Submit(fn)
	return nil
}

// Handles locking/unlocking mutex while appending result to results.
func (wp *WorkerPoolXT) appendResult(result *Result) {
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
	data      any
	error     error
	duration  time.Duration
	startedAt time.Time
}

func (j *Job) String() string {
	return fmt.Sprintf("Job: %s | Data: %v | Err: %v | Took: %s", j.Name, j.data, j.error, j.duration)
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
func recoverFromJobPanic(wp *WorkerPoolXT, j *Job) {
	if wp.stopped.Load() {
		return
	}
	if r := recover(); r != nil {
		j.duration = time.Since(j.startedAt)
		j.data = nil
		j.error = PanicRecoveryError{
			Message: fmt.Sprintf("Job recovered from panic \"%v\"", r),
			Job:     j,
		}
		wp.processJobsQueue <- j
		wp.resultsWaitGroup.Done()
	}
}
