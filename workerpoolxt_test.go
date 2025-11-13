package workerpoolxt

import (
	"context"
	"errors"

	//"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	backoff "github.com/cenkalti/backoff/v5"
	"github.com/gammazero/workerpool"
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

	helloWorldJob := &Job{
		Name: "Hello world job",
		Function: func() (any, error) {
			return "Hello, world!", nil
		},
	}

	// Submit job
	wp.SubmitXT(helloWorldJob)

	// Block main thread until all jobs are done.
	wp.StopWaitXT()

	//for _, r := range allJobResults {
	//	fmt.Printf("%+v", r)
	//}
}

func TestBasics(t *testing.T) {
	wp := New(5)
	wp.SubmitXT(&Job{
		Name: "a",
		Function: func() (any, error) {
			return true, nil
		},
	})
	wp.SubmitXT(&Job{
		Name: "b",
		Function: func() (any, error) {
			return true, nil
		},
	})
	wp.SubmitXT(&Job{
		Name: "c",
		Function: func() (any, error) {
			return nil, errors.New("err")
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

	wp.SubmitXT(&Job{
		Name: "cancellable-job",
		Function: func() (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			close(jobStarted) // signal job started

			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					close(jobCancelled) // signal cancellation observed
					return nil, ctx.Err()
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

	wp.SubmitXT(&Job{
		Name: "my ctx job",
		Function: func() (any, error) {
			timeout := time.Duration(100 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			e := SimulateLongRunningTaskWithContext(ctx, 10*time.Second)
			if e != nil {
				return nil, e
			}
			return "I could be anything", nil
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

	wp.SubmitXT(&Job{
		Name: "panic job",
		Function: func() (any, error) {
			panic("PANIC")
		},
	})

	results := wp.StopWaitXT()
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}
	if !errors.As(results[0].Error, &PanicRecoveryError{}) {
		t.Fatal("expected PanicRecoveryError : got", results[0].Error)
	}
}

func TestStopWait(t *testing.T) {
	wp := New(2)

	var mu sync.Mutex
	var ran []string

	wp.SubmitXT(&Job{
		Name: "job1",
		Function: func() (any, error) {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			ran = append(ran, "job1")
			mu.Unlock()
			return "done1", nil
		},
	})

	wp.SubmitXT(&Job{
		Name: "job2",
		Function: func() (any, error) {
			time.Sleep(200 * time.Millisecond)
			mu.Lock()
			ran = append(ran, "job2")
			mu.Unlock()
			return "done2", nil
		},
	})

	wp.resultsMutex.Lock()
	resultsBeforeStop := make([]*Result, len(wp.results))
	copy(resultsBeforeStop, wp.results)
	wp.resultsMutex.Unlock()
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

	err := wp.SubmitXT(&Job{
		Name: "cancellable job",
		Function: func() (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			close(jobStarted)

			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					close(jobCancelled)
					return nil, ctx.Err()
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

	wp.SubmitXT(&Job{
		Name: "retry-job",
		Function: func() (any, error) {
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
				return nil, err
			}
			return result, nil
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

func TestNativeStopWait(t *testing.T) {
	wp := New(4)

	wp.SubmitXT(&Job{
		Name: "native-stopwait",
		Function: func() (any, error) {
			time.Sleep(1 * time.Second)
			return true, nil
		},
	})

	wp.StopWait()
	allResults := wp.Results()
	if len(allResults) != 1 {
		t.Fatalf("expected 1 result got %d\n", len(allResults))
	}
}

func TestCallingResultsBeforeStopWait(t *testing.T) {
	wp := New(3)
	premature := wp.Results()
	if len(premature) > 0 {
		t.Fatal("expected 0 premature results")
	}
	wp.SubmitXT(&Job{
		Name: "Foo",
		Function: func() (any, error) {
			return true, nil
		},
	})
	wp.StopWait()
	r := wp.Results()
	if len(r) != 1 {
		t.Fatal("expected 1 result")
	}
}

func TestCallingStopWaitXTMoreThanOnce(t *testing.T) {
	wp := New(3)
	wp.SubmitXT(&Job{
		Name: "before",
		Function: func() (any, error) {
			return true, nil
		},
	})

	/*firstResults :=*/
	wp.StopWaitXT()

	wp.SubmitXT(&Job{
		Name: "after",
		Function: func() (any, error) {
			return true, nil
		},
	})

	/*secondResults :=*/
	wp.StopWaitXT()
	//fmt.Println("firstResults ", firstResults)
	//fmt.Println("secondResults", secondResults)
}

func TestNilFunctionInJob(t *testing.T) {
	wp := New(2)
	wp.SubmitXT(&Job{
		Name:     "nil func",
		Function: nil,
	})
	/*res :=*/ wp.StopWaitXT()
	//fmt.Println("res", res)
}

func TestWithWorkerPool(t *testing.T) {
	existing := workerpool.New(5)
	wp := WithWorkerPool(existing)

	wp.SubmitXT(&Job{
		Name: "WithWorkerPool",
		Function: func() (any, error) {
			return "yup", nil
		},
	})

	res := wp.StopWaitXT()

	if len(res) != 1 {
		t.Fatal("expected 1 result")
	}
}
