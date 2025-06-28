package workerpoolxt

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	backoff "github.com/cenkalti/backoff/v5"
)

/**
 *
 * Any test ending in `_Special` will have the following command ran against it:
 *
 *   > go test -race -run ^Test.*Special$ -count=2000
 *
 */

var (
	defaultTimeout = time.Duration(time.Second * 10)
	freshCtx       = func() context.Context { return context.Background() }
	defaultWorkers = 10
)

func SleepForWithContext(ctx context.Context, sleepFor time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(sleepFor):
		return nil
	}
}

func SimulateLongRunningTaskWithContext(ctx context.Context, runFor time.Duration) error {
	step := time.Millisecond
	ticker := time.NewTicker(step)
	defer ticker.Stop()

	elapsed := time.Duration(0)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			elapsed += step
			if elapsed >= runFor {
				return nil
			}
		}
	}
}

func TestHelloWorldFromREADME(t *testing.T) {
	numWorkers := 5
	wp := New(numWorkers)

	helloWorldJob := Job{
		Name: "Hello world job",
		Function: func() Result {
			return Result{
				Error: nil,             // not required
				Data:  "Hello, world!", // type `any`
			}
		},
	}

	// Submit job
	wp.SubmitXT(helloWorldJob)

	// Block main thread until all jobs are done.
	allJobResults := wp.StopWaitXT()

	for _, r := range allJobResults {
		fmt.Printf("Name: %s\nData: %v\nDuration: %v\nError?: %v", r.Name(), r.Data, r.Duration(), r.Error)
	}
}

func TestBasics(t *testing.T) {
	wp := New(defaultWorkers)
	wp.SubmitXT(Job{
		Name: "a",
		Function: func() Result {
			return Result{Data: true}
		},
	})
	wp.SubmitXT(Job{
		Name: "b",
		Function: func() Result {
			return Result{Data: true}
		},
	})
	wp.SubmitXT(Job{
		Name: "c",
		Function: func() Result {
			return Result{Error: errors.New("err")}
		},
	})

	results := wp.StopWaitXT()

	if len(results) != 3 {
		t.Fatalf("Expected 3 results got : %d", len(results))
	}
}

func TestWorkerPoolCancelsJobEarly(t *testing.T) {
	wp := New(1)

	var jobStarted = make(chan struct{})
	var jobCancelled = make(chan struct{})

	wp.SubmitXT(Job{
		Name: "cancellable-job",
		Function: func() Result {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			close(jobStarted) // signal job started

			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					close(jobCancelled) // signal cancellation observed
					return Result{Error: ctx.Err()}
				case <-ticker.C:
					// simulate work chunk
				}
			}
		},
	})

	// Wait until job started
	<-jobStarted

	// Wait for cancellation to be observed or timeout after 500ms
	select {
	case <-jobCancelled:
		// success, job was cancelled early
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Job did not observe cancellation in time")
	}

	results := wp.StopWaitXT()
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, context.DeadlineExceeded) {
		t.Fatalf("Expected DeadlineExceeded error, got %v", results[0].Error)
	}
}

func TestTimeouts(t *testing.T) {
	numWorkers := 10
	wp := New(numWorkers)

	wp.SubmitXT(Job{
		Name: "my ctx job",
		Function: func() Result {
			timeout := time.Duration(100 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			e := SimulateLongRunningTaskWithContext(ctx, 10*time.Second)
			if e != nil {
				return Result{Error: e}
			}
			return Result{Data: "I could be anything"}
		},
	})

	results := wp.StopWaitXT()
	if len(results) != 1 {
		t.Fatalf("Expected 1 result : got %d", len(results))
	}
	if !errors.Is(results[0].Error, context.DeadlineExceeded) {
		t.Fatalf("Expected err %s : got err %s", context.DeadlineExceeded, results[0].Error)
	}
}

func TestJobPanicRecovery(t *testing.T) {
	wp := New(1)

	wp.SubmitXT(Job{
		Name: "panic job",
		Function: func() Result {
			panic("job panicked!")
		},
	})

	results := wp.StopWaitXT()
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}
	if !errors.As(results[0].Error, &JobPanicError{}) {
		t.Fatal("expected JobPanicError : got", results[0].Error)
	}
}

func TestStopWait(t *testing.T) {
	wp := New(2)

	var mu sync.Mutex
	var ran []string

	wp.SubmitXT(Job{
		Name: "job1",
		Function: func() Result {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			ran = append(ran, "job1")
			mu.Unlock()
			return Result{Data: "done1"}
		},
	})

	wp.SubmitXT(Job{
		Name: "job2",
		Function: func() Result {
			time.Sleep(200 * time.Millisecond)
			mu.Lock()
			ran = append(ran, "job2")
			mu.Unlock()
			return Result{Data: "done2"}
		},
	})

	resultsBeforeStop := wp.results
	wp.StopWaitXT()

	mu.Lock()
	defer mu.Unlock()
	if len(ran) != 2 {
		t.Fatalf("expected 2 jobs ran, got %d", len(ran))
	}
	if len(resultsBeforeStop) == len(wp.results) {
		t.Fatalf("expected results to grow after StopWait")
	}
}

func TestSubmitXT_ContextCancellation(t *testing.T) {
	wp := New(1)

	jobStarted := make(chan struct{})
	jobCancelled := make(chan struct{})

	err := wp.SubmitXT(Job{
		Name: "cancellable job",
		Function: func() Result {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			close(jobStarted)

			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					close(jobCancelled)
					return Result{Error: ctx.Err()}
				case <-ticker.C:
					// where work would be
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("unexpected error from SubmitXT: %v", err)
	}

	select {
	case <-jobStarted:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}

	select {
	case <-jobCancelled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("job did not cancel in time")
	}

	results := wp.StopWaitXT()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded error, got %v", results[0].Error)
	}
}

func TestJobRetry(t *testing.T) {
	wp := New(10)
	maxRetry := 3
	numRetry := 0

	wp.SubmitXT(Job{
		Name: "retry-job",
		Function: func() Result {
			workToRetry := func() (string, error) {
				if numRetry < maxRetry {
					o, e := "", errors.New("failed "+strconv.Itoa(numRetry))
					numRetry++
					return o, e
				}
				return "Done", nil
			}

			result, err := backoff.Retry(context.Background(), workToRetry, backoff.WithBackOff(backoff.NewExponentialBackOff()))
			if err != nil {
				return Result{Error: err}
			}
			return Result{Data: result}
		},
	})

	results := wp.StopWaitXT()
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}
	if numRetry != maxRetry {
		t.Fatal("expected to hit max retries before succeeding")
	}
	if results[0].Error != nil {
		t.Fatal("expected no error in job")
	}
}
