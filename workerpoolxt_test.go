package workerpoolxt

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

const max = 20

func TestResults(t *testing.T) {
	defer goleak.VerifyNone(t)

	jobs := 10
	wp := New(2)

	for i := 0; i < jobs; i++ {
		wp.Submit(&Job{
			Name: "Job " + strconv.Itoa(i),
			Function: func() (any, error) {
				return i, nil
			},
		})
	}

	wp.StopWait()
	res := wp.Results()
	if len(res) != jobs {
		t.Fatalf("Expected %d results got %d\n", jobs, len(res))
	}
}

func TestResultsPauseJobsAreIgnored(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(2)

	wp.Submit(&Job{
		Name: "Some Result",
		Function: func() (any, error) {
			myData := 1234
			return myData, nil
		},
	})

	wp.Submit(&Job{
		Name: "Some Result With Error",
		Function: func() (any, error) {
			return nil, errors.New("something went wrong!")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())

	wp.Pause(ctx)
	cancel()
	wp.StopWait()

	res := wp.Results()
	if len(res) != 2 {
		t.Fatalf("Expected 2 results got %d\n", len(res))
	}
}

// go test -run TestLongRunningJobsHaveResultsProcessed -race
func TestLongRunningJobsHaveResultsProcessed(t *testing.T) {
	wp := New(5)
	jobs := 10

	for i := 0; i < jobs; i++ {
		wp.Submit(&Job{
			Name: "Job " + strconv.Itoa(i),
			Function: func() (any, error) {
				time.Sleep(1 * time.Second)
				return true, nil
			},
		})
	}

	wp.StopWait()

	num_results := len(wp.Results())
	if num_results != jobs {
		t.Fatalf("Expected %d results, got %d\n", jobs, num_results)
	}
}

func TestExample(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(2)
	requests := []string{"alpha", "beta", "gamma", "delta", "epsilon"}

	rspChan := make(chan string, len(requests))
	for _, r := range requests {
		r := r
		wp.Submit(&Job{
			Name: "Test example",
			Function: func() (any, error) {
				rspChan <- r
				return nil, nil
			},
		})
	}

	wp.StopWait()

	close(rspChan)
	rspSet := map[string]struct{}{}
	for rsp := range rspChan {
		rspSet[rsp] = struct{}{}
	}
	if len(rspSet) < len(requests) {
		t.Fatal("Did not handle all requests")
	}
	for _, req := range requests {
		if _, ok := rspSet[req]; !ok {
			t.Fatal("Missing expected values:", req)
		}
	}
}

func TestMaxWorkers(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(0)
	wp.Stop()
	if wp.maxWorkers != 1 {
		t.Fatal("should have created one worker")
	}

	wp = New(max)
	defer wp.Stop()

	if wp.Size() != max {
		t.Fatal("wrong size returned")
	}

	started := make(chan struct{}, max)
	release := make(chan struct{})

	// Start workers, and have them all wait on a channel before completing.
	for i := 0; i < max; i++ {
		wp.Submit(&Job{
			Name: "Test Max Workers",
			Function: func() (any, error) {
				started <- struct{}{}
				<-release
				return nil, nil
			},
		})
	}

	// Wait for all queued tasks to be dispatched to workers.
	if wp.waitingQueue.Len() != wp.WaitingQueueSize() {
		t.Fatal("Working Queue size returned should not be 0")
	}
	timeout := time.After(5 * time.Second)
	for startCount := 0; startCount < max; {
		select {
		case <-started:
			startCount++
		case <-timeout:
			t.Fatal("timed out waiting for workers to start")
		}
	}

	// Release workers.
	close(release)
}

func TestReuseWorkers(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(5)
	defer wp.Stop()

	release := make(chan struct{})

	// Cause worker to be created, and available for reuse before next task.
	for i := 0; i < 10; i++ {
		wp.Submit(&Job{Name: "TestReuseWorkers_" + strconv.Itoa(i), Function: func() (any, error) {
			<-release
			return nil, nil
		}})
		release <- struct{}{}
		time.Sleep(time.Millisecond)
	}
	close(release)

	// If the same worker was always reused, then only one worker would have
	// been created and there should only be one ready.
	if countReady(wp) > 1 {
		t.Fatal("Worker not reused")
	}
}

func TestResultsRace(t *testing.T) {
	wp := New(5)
	jobs := 50

	// Start a go routine that does nothing but read results
	killResultsReader := make(chan struct{})
	go func() {
	Loop:
		for {
			select {
			case _, ok := <-killResultsReader:
				if !ok {
					break Loop
				}
			default:
				wp.Results()
			}
		}
	}()

	for i := 0; i < jobs; i++ {
		wp.Submit(&Job {
			Name: "Job " + strconv.Itoa(i),
			Function: func() (any, error) {
				time.Sleep(500 * time.Millisecond)
				return "Job " + strconv.Itoa(i), nil
			},
		})
	}

	wp.StopWait()
	close(killResultsReader)

	resLen := len(wp.Results())
	if resLen != jobs {
		t.Fatalf("Expected %d results, got %d\n", jobs, resLen)
	}
}

func TestWorkerTimeout(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(max)
	defer wp.Stop()

	// Start workers, and have them all wait on ctx before completing.
	ctx, cancel := context.WithCancel(context.Background())
	wp.Pause(ctx)

	if anyReady(wp) {
		t.Fatal("number of ready workers should be zero")
	}

	if wp.killIdleWorker() {
		t.Fatal("should have been no idle workers to kill")
	}

	// Release workers.
	cancel()

	if countReady(wp) != max {
		t.Fatal("Expected", max, "ready workers")
	}

	// Check that a worker timed out.
	time.Sleep(idleTimeout*2 + idleTimeout/2)
	if countReady(wp) != max-1 {
		t.Fatal("First worker did not timeout")
	}

	// Check that another worker timed out.
	time.Sleep(idleTimeout)
	if countReady(wp) != max-2 {
		t.Fatal("Second worker did not timeout")
	}
}

func TestStop(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(max)

	// Start workers, and have them all wait on ctx before completing.
	ctx, cancel := context.WithCancel(context.Background())
	wp.Pause(ctx)

	// Release workers.
	cancel()

	if wp.Stopped() {
		t.Fatal("pool should not be stopped")
	}

	wp.Stop()

	if anyReady(wp) {
		t.Fatal("should have zero workers after stop")
	}

	if !wp.Stopped() {
		t.Fatal("pool should be stopped")
	}

	// Start workers, and have them all wait on a channel before completing.

	wp = New(5)

	release := make(chan struct{})
	finished := make(chan struct{}, max)

	for i := 0; i < max; i++ {
		wp.Submit(&Job{
			Name: "Test Stop",
			Function: func() (any, error) {
				<-release
				finished <- struct{}{}
				return nil, nil
			},
		})
	}

	// Call Stop() and see that only the already running tasks were completed.
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(release)
	}()

	wp.Stop()

	var count int
Count:
	for count < max {
		select {
		case <-finished:
			count++
		default:
			break Count
		}
	}
	if count > 5 {
		t.Fatal("Should not have completed any queued tasks, did", count)
	}
	// Check that calling Stop() again is OK.
	wp.Stop()
}

func TestStopWait(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Start workers, and have them all wait on a channel before completing.
	wp := New(5)
	release := make(chan struct{})
	finished := make(chan struct{}, max)
	for i := 0; i < max; i++ {
		wp.Submit(&Job{
			Name: "TestStopWait",
			Function: func() (any, error) {
				<-release
				finished <- struct{}{}
				return nil, nil
			},
		})
	}

	// Call StopWait() and see that all tasks were completed.
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(release)
	}()
	wp.StopWait()
	for count := 0; count < max; count++ {
		select {
		case <-finished:
		default:
			t.Fatal("Should have completed all queued tasks")
		}
	}

	if anyReady(wp) {
		t.Fatal("should have zero workers after stopwait")
	}

	if !wp.Stopped() {
		t.Fatal("pool should be stopped")
	}

	// Make sure that calling StopWait() with no queued tasks is OK.
	wp = New(5)
	wp.StopWait()

	if anyReady(wp) {
		t.Fatal("should have zero workers after stopwait")
	}

	// Check that calling StopWait() again is OK.
	wp.StopWait()
}

func TestSubmitWait(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(1)
	defer wp.Stop()

	// Check that these are noop.
	wp.Submit(nil)
	wp.SubmitWait(nil)

	done1 := make(chan struct{})
	wp.Submit(&Job{
		Name: "TestSubmitWait_1",
		Function: func() (any, error) {
			time.Sleep(100 * time.Millisecond)
			close(done1)
			return nil, nil
		},
	})
	select {
	case <-done1:
		t.Fatal("Submit did not return immediately")
	default:
	}

	done2 := make(chan struct{})
	wp.SubmitWait(&Job{
		Name: "TestSubmitWait_2",
		Function: func() (any, error) {
			time.Sleep(100 * time.Millisecond)
			close(done2)
			return nil, nil
		},
	})
	select {
	case <-done2:
	default:
		t.Fatal("SubmitWait did not wait for function to execute")
	}
}

func TestOverflow(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(2)
	defer wp.Stop()
	releaseChan := make(chan struct{})

	// Start workers, and have them all wait on a channel before completing.
	for i := 0; i < 64; i++ {
		wp.Submit(&Job{Name: "TestOverflow", Function: func() (any, error) { <-releaseChan; return nil, nil }})
	}

	// Start a goroutine to free the workers after calling stop.  This way
	// the dispatcher can exit, then when this goroutine runs, the workerpool
	// can exit.
	go func() {
		<-time.After(time.Millisecond)
		close(releaseChan)
	}()
	wp.Stop()

	// Now that the worker pool has exited, it is safe to inspect its waiting
	// queue without causing a race.
	qlen := wp.waitingQueue.Len()
	if qlen != 62 {
		t.Fatal("Expected 62 tasks in waiting queue, have", qlen)
	}
}

func TestStopRace(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(max)
	defer wp.Stop()

	workRelChan := make(chan struct{})

	var started sync.WaitGroup
	started.Add(max)

	// Start workers, and have them all wait on a channel before completing.
	for i := 0; i < max; i++ {
		wp.Submit(&Job{
			Name: "TestStopRace",
			Function: func() (any, error) {
				started.Done()
				<-workRelChan
				return nil, nil
			},
		})
	}

	started.Wait()

	const doneCallers = 5
	stopDone := make(chan struct{}, doneCallers)
	for i := 0; i < doneCallers; i++ {
		go func() {
			wp.Stop()
			stopDone <- struct{}{}
		}()
	}

	select {
	case <-stopDone:
		t.Fatal("Stop should not return in any goroutine")
	default:
	}

	close(workRelChan)

	timeout := time.After(time.Second)
	for i := 0; i < doneCallers; i++ {
		select {
		case <-stopDone:
		case <-timeout:
			wp.Stop()
			t.Fatal("timedout waiting for Stop to return")
		}
	}
}

// Run this test with race detector to test that using WaitingQueueSize has no
// race condition
// `go test -run TestWaitingQueueSizeRace -race`
func TestWaitingQueueSizeRace(t *testing.T) {
	defer goleak.VerifyNone(t)
	const (
		goroutines = 10
		tasks      = 20
		workers    = 5
	)
	wp := New(workers)
	defer wp.Stop()

	maxChan := make(chan int)
	for g := 0; g < goroutines; g++ {
		go func() {
			max := 0
			// Submit 100 tasks, checking waiting queue size each time.  Report
			// the maximum queue size seen.
			for i := 0; i < tasks; i++ {
				wp.Submit(&Job{
					Name: "TestWaitingQueueSizeRace",
					Function: func() (any, error) {
						time.Sleep(time.Microsecond)
						return nil, nil
					},
				})
				waiting := wp.WaitingQueueSize()
				if waiting > max {
					max = waiting
				}
			}
			maxChan <- max
		}()
	}

	// Find maximum queuesize seen by any goroutine.
	maxMax := 0
	for g := 0; g < goroutines; g++ {
		max := <-maxChan
		if max > maxMax {
			maxMax = max
		}
	}
	if maxMax == 0 {
		t.Error("expected to see waiting queue size > 0")
	}
	if maxMax >= goroutines*tasks {
		t.Error("should not have seen all tasks on waiting queue")
	}
}

func TestPause(t *testing.T) {
	defer goleak.VerifyNone(t)

	wp := New(25)
	defer wp.Stop()

	ctx, cancel := context.WithCancel(context.Background())

	ran := make(chan struct{})
	wp.Submit(&Job{
		Name: "TestPause_1",
		Function: func() (any, error) {
			time.Sleep(time.Millisecond)
			close(ran)
			return nil, nil
		},
	})

	wp.Pause(ctx)

	// Check that Pause waits for all previously submitted tasks to run.
	select {
	case <-ran:
	default:
		t.Error("did not run all tasks before returning from Pause")
	}

	ran = make(chan struct{})
	wp.Submit(&Job{
		Name: "TestPause_2",
		Function: func() (any, error) {
			close(ran)
			return nil, nil
		},
	})

	// Check that a new task did not run while paused
	select {
	case <-ran:
		t.Error("ran while paused")
	case <-time.After(time.Millisecond):
	}

	// Check that task was enqueued
	if wp.WaitingQueueSize() != 1 {
		t.Error("waiting queue size should be 1")
	}

	// Cancel context to unpause workers.
	cancel()

	// Check that task was run after unpausing.
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Error("did not run after canceling pause")
	}

	// ---- Test pause while paused

	ctx, cancel = context.WithCancel(context.Background())
	wp.Pause(ctx)

	ctx2, cancel2 := context.WithCancel(context.Background())

	pauseDone := make(chan struct{})
	go func() {
		wp.Pause(ctx2)
		close(pauseDone)
	}()

	// Check that second pause does not return until first pause in canceled
	select {
	case <-pauseDone:
		wp.Stop()
		t.Fatal("second Pause should not have returned")
	case <-time.After(time.Millisecond):
	}

	cancel() // cancel 1st pause

	// Check that second pause returns
	select {
	case <-pauseDone:
	case <-time.After(time.Second):
		wp.Stop()
		t.Fatal("timed out waiting for Pause to return")
	}

	cancel2() // cancel 2nd pause

	// ---- Test concurrent pauses

	ctx, cancel = context.WithCancel(context.Background())
	ctx2, cancel2 = context.WithCancel(context.Background())
	pauseDone = make(chan struct{})
	pause2Done := make(chan struct{})
	go func() {
		wp.Pause(ctx)
		close(pauseDone)
	}()
	go func() {
		wp.Pause(ctx2)
		close(pause2Done)
	}()

	select {
	case <-pauseDone:
		cancel()
		<-pause2Done
		cancel2()
	case <-pause2Done:
		cancel2()
		<-pauseDone
		cancel()
	case <-time.After(time.Second):
		t.Fatal("concurrent pauses deadlocked")
	}

	// ---- Test stopping paused pool ----

	ctx, cancel = context.WithCancel(context.Background())
	ctx2, cancel2 = context.WithCancel(context.Background())

	// Stack up two pauses
	wp.Pause(ctx)
	go wp.Pause(ctx2)

	ran = make(chan struct{})
	wp.Submit(&Job{
		Name: "TestPause_3",
		Function: func() (any, error) {
			close(ran)
			return nil, nil
		},
	})

	stopDone := make(chan struct{})
	go func() {
		wp.StopWait()
		close(stopDone)
	}()

	// Check that task was run after calling StopWait
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for StopWait to return")
	}

	// Check that task was run after calling StopWait
	select {
	case <-ran:
	default:
		t.Error("did not run after canceling pause")
	}

	defer cancel()
	defer cancel2()

	// ---- Test pause after stop ----

	ctx, cancel = context.WithCancel(context.Background())
	pauseDone = make(chan struct{})
	go func() {
		wp.Pause(ctx)
		close(pauseDone)
	}()
	select {
	case <-pauseDone:
	case <-time.After(time.Second):
		t.Fatal("pause after stop did not return")
	}
	cancel()
}

func TestWorkerLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	const workerCount = 100

	wp := New(workerCount)

	// Start workers, and have them all wait on a channel before completing.
	for i := 0; i < workerCount; i++ {
		wp.Submit(&Job{
			Name: "TestWorkerLeak",
			Function: func() (any, error) {
				time.Sleep(time.Millisecond)
				return nil, nil
			},
		})
	}

	// If wp..Stop() is not waiting for all workers to complete, then goleak
	// should catch that
	wp.Stop()
}

func anyReady(w *WorkerPool) bool {
	release := make(chan struct{})
	wait := &Job{
		Name: "anyReady",
		Function: func() (any, error) {
			<-release
			return nil, nil
		},
	}
	select {
	case w.workerQueue <- wait:
		close(release)
		return true
	default:
	}
	return false
}

func countReady(w *WorkerPool) int {
	// Try to stop max workers.
	timeout := time.After(100 * time.Millisecond)
	release := make(chan struct{})
	var readyCount int
	for i := 0; i < max; i++ {
		select {
		case w.workerQueue <- &Job{
			Name: "countReady",
			Function: func() (any, error) {
				<-release
				return nil, nil
			},
			ignoreResult: true,
		}:
			readyCount++
		case <-timeout:
			i = max
		}
	}

	close(release)
	return readyCount
}

/*

Run benchmarking with: go test -bench '.'

*/

func BenchmarkEnqueue(b *testing.B) {
	wp := New(1)
	defer wp.Stop()
	releaseChan := make(chan struct{})

	b.ResetTimer()

	// Start workers, and have them all wait on a channel before completing.
	for i := 0; i < b.N; i++ {
		wp.Submit(&Job{
			Name: "BenchmarkEnqueue",
			Function: func() (any, error) {
				<-releaseChan
				return nil, nil
			},
			ignoreResult: true,
		})
	}
	close(releaseChan)
}

func BenchmarkEnqueue2(b *testing.B) {
	wp := New(2)
	defer wp.Stop()

	b.ResetTimer()

	// Start workers, and have them all wait on a channel before completing.
	for i := 0; i < b.N; i++ {
		releaseChan := make(chan struct{})
		for i := 0; i < 64; i++ {
			wp.Submit(&Job{
				Name: "BenchmarkEnqueue2",
				Function: func() (any, error) {
					<-releaseChan
					return nil, nil
				},
				ignoreResult: true,
			})
		}
		close(releaseChan)
	}
}

func BenchmarkExecute1Worker(b *testing.B) {
	benchmarkExecWorkers(1, b)
}

func BenchmarkExecute2Worker(b *testing.B) {
	benchmarkExecWorkers(2, b)
}

func BenchmarkExecute4Workers(b *testing.B) {
	benchmarkExecWorkers(4, b)
}

func BenchmarkExecute16Workers(b *testing.B) {
	benchmarkExecWorkers(16, b)
}

func BenchmarkExecute64Workers(b *testing.B) {
	benchmarkExecWorkers(64, b)
}

func BenchmarkExecute1024Workers(b *testing.B) {
	benchmarkExecWorkers(1024, b)
}

func benchmarkExecWorkers(n int, b *testing.B) {
	wp := New(n)
	defer wp.Stop()
	var allDone sync.WaitGroup
	allDone.Add(b.N * n)

	b.ResetTimer()

	// Start workers, and have them all wait on a channel before completing.
	for i := 0; i < b.N; i++ {
		for j := 0; j < n; j++ {
			wp.Submit(&Job{
				Name: "benchmarkExecWorkers",
				Function: func() (any, error) {
					//time.Sleep(100 * time.Microsecond)
					allDone.Done()
					return nil, nil
				},
			})
		}
	}
	allDone.Wait()
}
