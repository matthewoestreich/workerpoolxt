package generic

import (
	"context"
	"errors"
	//"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	backoff "github.com/cenkalti/backoff/v5"
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

type ReturnTypes struct {
	String string
	Bool   bool
}

func TestCodeFromREADME(t *testing.T) {
	// First define possible return types for any given job.
	type WebResult struct {
		Response string
	}
	type SomeResult struct {
		Success bool
		Message string
	}
	// etc...

	// Create a "container' type to hold possible return types.
	type Output struct {
		Web    *WebResult
		Some   *SomeResult
		String string
	}

	// Use "container" type as generic
	pool := New[Output](5)

	helloWorldJob := &Job[Output]{
		Name: "Hello world job",
		// Function signature must be |func() (T, error)|
		Function: func() (Output, error) {
			//var pretendError error = nil
			//if pretendError != nil {
			//	// To demonstrate how you would return an error
			//	return Output{}, pretendError
			//}
			return Output{String: "Hello, world!"}, nil
		},
	}

	someJob := &Job[Output]{
		Name: "Some job",
		Function: func() (Output, error) {
			someResult := &SomeResult{
				Success: true,
				Message: "we did the thing!",
			}
			return Output{Some: someResult}, nil
		},
	}

	webJob := &Job[Output]{
		Name: "Web job",
		Function: func() (Output, error) {
			webResult := &WebResult{Response: "that thing, we did it!"}
			return Output{Web: webResult}, nil
		},
	}

	jobs := []*Job[Output]{
		helloWorldJob,
		someJob,
		webJob,
	}

	// Submit jobs
	for _, job := range jobs {
		pool.SubmitXT(job)
	}

	// Block main thread until all jobs are done.
	results := pool.StopWaitXT()
	// Results also accessable via
	//sameResults := pool.Results()

	if len(results) != len(jobs) {
		t.Fatal("num results != num jobs")
	}

	// Now you can access results in a type-safe manner
	for _, result := range results {
		if result.Data.Some != nil {
			// Most likely a job that returned 'SomeJob' type.
			//isSuccess := result.Data.Some.Success
			//message := result.Data.Some.Message
			//jobName := result.Name
			//jobDuration := result.Duration
			//jobError := result.Error
			//fmt.Println(jobName, jobDuration, jobError, message, isSuccess)
		}
		if result.Data.Web != nil {
			// Handle 'WebResult' job
		}
		if result.Data.String != "" {
			// Handle basic string result...
		}
	}
}

func TestBasics(t *testing.T) {
	wp := New[ReturnTypes](5)
	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "a",
		Function: func() (ReturnTypes, error) {
			return ReturnTypes{Bool: true}, nil
		},
	})
	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "b",
		Function: func() (ReturnTypes, error) {
			return ReturnTypes{Bool: true}, nil
		},
	})
	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "c",
		Function: func() (ReturnTypes, error) {
			return ReturnTypes{}, errors.New("err")
		},
	})

	results := wp.StopWaitXT()

	if len(results) != 3 {
		t.Fatalf("Expected 3 results got : %d", len(results))
	}
}

func TestWorkerPoolCancelsJobEarly(t *testing.T) {
	wp := New[ReturnTypes](1)

	var jobStarted = make(chan struct{})
	var jobCancelled = make(chan struct{})

	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "cancellable-job",
		Function: func() (ReturnTypes, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			close(jobStarted) // signal job started

			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					close(jobCancelled) // signal cancellation observed
					return ReturnTypes{}, ctx.Err()
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
	wp := New[ReturnTypes](numWorkers)

	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "my ctx job",
		Function: func() (ReturnTypes, error) {
			timeout := time.Duration(100 * time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			e := SimulateLongRunningTaskWithContext(ctx, 10*time.Second)
			if e != nil {
				return ReturnTypes{}, e
			}
			return ReturnTypes{String: "I could be anything"}, nil
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
	wp := New[ReturnTypes](1)

	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "panic job",
		Function: func() (ReturnTypes, error) {
			panic("PANIC")
		},
	})

	results := wp.StopWaitXT()
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}
	if !errors.As(results[0].Error, &PanicRecoveryError[ReturnTypes]{}) {
		t.Fatal("expected PanicRecoveryError : got", results[0].Error)
	}
}

func TestStopWait(t *testing.T) {
	wp := New[ReturnTypes](2)

	var mu sync.Mutex
	var ran []string

	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "job1",
		Function: func() (ReturnTypes, error) {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			ran = append(ran, "job1")
			mu.Unlock()
			return ReturnTypes{String: "done1"}, nil
		},
	})

	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "job2",
		Function: func() (ReturnTypes, error) {
			time.Sleep(200 * time.Millisecond)
			mu.Lock()
			ran = append(ran, "job2")
			mu.Unlock()
			return ReturnTypes{String: "done2"}, nil
		},
	})

	wp.resultsMutex.Lock()
	resultsBeforeStop := make([]*Result[ReturnTypes], len(wp.results))
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
	wp := New[ReturnTypes](1)

	jobStarted := make(chan struct{})
	jobCancelled := make(chan struct{})

	err := wp.SubmitXT(&Job[ReturnTypes]{
		Name: "cancellable job",
		Function: func() (ReturnTypes, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			close(jobStarted)

			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					close(jobCancelled)
					return ReturnTypes{}, ctx.Err()
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
	wp := New[ReturnTypes](10)
	maxRetry := 3
	numRetry := 0

	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "retry-job",
		Function: func() (ReturnTypes, error) {
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
				return ReturnTypes{}, err
			}
			return ReturnTypes{String: result}, nil
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
	wp := New[ReturnTypes](4)

	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "native-stopwait",
		Function: func() (ReturnTypes, error) {
			time.Sleep(1 * time.Second)
			return ReturnTypes{Bool: true}, nil
		},
	})

	wp.StopWait()
	allResults := wp.Results()
	if len(allResults) != 1 {
		t.Fatal("expected 1 result")
	}
}

func TestCallingResultsBeforeStopWait(t *testing.T) {
	wp := New[ReturnTypes](3)
	premature := wp.Results()
	if len(premature) > 0 {
		t.Fatal("expected 0 premature results")
	}
	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "Foo",
		Function: func() (ReturnTypes, error) {
			return ReturnTypes{Bool: true}, nil
		},
	})
	wp.StopWait()
	r := wp.Results()
	if len(r) != 1 {
		t.Fatal("expected 1 result")
	}
}

func TestCallingStopWaitXTMoreThanOnce(t *testing.T) {
	wp := New[ReturnTypes](3)
	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "before",
		Function: func() (ReturnTypes, error) {
			return ReturnTypes{Bool: true}, nil
		},
	})
	/*firstResults :=*/ wp.StopWaitXT()
	wp.SubmitXT(&Job[ReturnTypes]{
		Name: "after",
		Function: func() (ReturnTypes, error) {
			return ReturnTypes{Bool: true}, nil
		},
	})
	/*secondResults :=*/ wp.StopWaitXT()
	//fmt.Println("firstResults ", firstResults)
	//fmt.Println("secondResults", secondResults)
}

func TestNilFunctionInJob(t *testing.T) {
	wp := New[ReturnTypes](2)
	wp.SubmitXT(&Job[ReturnTypes]{
		Name:     "nil func",
		Function: nil,
	})
	/*res :=*/ wp.StopWaitXT()
	//fmt.Println("res", res)
}

func TestGenericResults(t *testing.T) {
	type WebResult struct {
		Response string
	}
	type Bar struct {
		BarMember string
	}
	type FooResult struct {
		Something string
		Bar       Bar
	}

	type Output struct {
		Web    *WebResult
		Foo    *FooResult
		String string
	}

	wp := New[Output](5)

	wp.SubmitXT(&Job[Output]{
		Name: "Web Job",
		Function: func() (Output, error) {
			webResult := &WebResult{Response: "some data from some website"}
			return Output{Web: webResult}, nil
		},
	})
	wp.SubmitXT(&Job[Output]{
		Name: "Foo Job",
		Function: func() (Output, error) {
			foo := &FooResult{
				Bar: Bar{BarMember: "Foo->Bar->BarMember"},
			}
			return Output{Foo: foo}, nil
		},
	})
	wp.SubmitXT(&Job[Output]{
		Name: "String Job",
		Function: func() (Output, error) {
			return Output{String: "hello"}, nil
		},
	})

	webJobs := 0
	fooJobs := 0
	strJobs := 0

	for _, result := range wp.StopWaitXT() {
		d := result.Data
		if d.Foo != nil && d.Web == nil && d.String == "" {
			// Results for a job that returned "Foo"
			fooJobs++
		}
		if d.Web != nil && d.Foo == nil && d.String == "" {
			webJobs++
		}
		if d.String != "" && d.Foo == nil && d.Web == nil {
			strJobs++
		}
	}

	if webJobs != 1 || fooJobs != 1 || strJobs != 1 {
		t.Fatal("expected 1 Web job and 1 Foo job and 1 string job")
	}
}
