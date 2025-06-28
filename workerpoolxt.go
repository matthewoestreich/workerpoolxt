package workerpoolxt

import (
	"errors"
	"sync"
	"time"

	"github.com/gammazero/workerpool"
)

// WorkerpoolXT wraps workerpool
type WorkerpoolXT struct {
	*workerpool.WorkerPool
	resultsMutex sync.Mutex
	results      []Result
	kill         chan struct{}
	once         sync.Once
}

// New creates a new workerpoolxt
func New(size int) *WorkerpoolXT {
	return &WorkerpoolXT{
		WorkerPool: workerpool.New(size),
		results:    []Result{},
		kill:       make(chan struct{}),
	}
}

// SubmitXT submits a Job to workerpool
func (xt *WorkerpoolXT) SubmitXT(job Job) error {
	if job.Function == nil {
		return errors.New("job.Function is nil")
	}

	xt.Submit(func() {
		job.startedAt = time.Now()
		resultChan := make(chan Result, 1)

		go func() {
			defer recoverFromJobPanic(job, resultChan)
			res := job.Function()
			res.name = job.Name
			res.duration = time.Since(job.startedAt)
			resultChan <- res
		}()

		res := <-resultChan
		xt.resultsMutex.Lock()
		xt.results = append(xt.results, res)
		xt.resultsMutex.Unlock()
	})

	return nil
}

// StopWaitXT blocks main thread and waits for all jobs
func (xt *WorkerpoolXT) StopWaitXT() []Result {
	xt.once.Do(func() {
		xt.StopWait()
		close(xt.kill)
	})
	return xt.results
}

// Helper function to recover from a panic within a job
func recoverFromJobPanic(j Job, rc chan<- Result) {
	if r := recover(); r != nil {
		rc <- Result{
			Error:    JobPanicError{Job: j, Message: "job panic"},
			name:     j.Name,
			duration: time.Since(j.startedAt),
		}
	}
}