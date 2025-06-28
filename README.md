<h1 align="center">workerpoolxt</h1>
<p align="center">
  Worker pool library that extends <a href="https://github.com/gammazero/workerpool">https://github.com/gammazero/workerpool</a>
</p>
<p align="center"><code>go get github.com/matthewoestreich/workerpoolxt</code></p>

---

## Features

We wrap the func(s) that get passed to `workerpool`, which we call "jobs".

You can collect results from each job (errors included), give each job a name, get the duration each job ran for, plus more.

## Import

```go
import wpxt "github.com/matthewoestreich/workerpoolxt"
```

## Examples

### Hello, world!

```go
numWorkers := 5
pool := wpxt.New(numWorkers)

helloWorldJob := wpxt.Job{
  Name: "Hello world job",
  Function: func() wpxt.Result {
    return wpxt.Result{Data: "Hello, world!", Error: nil}
  },
}

// Submit job
pool.SubmitXT(helloWorldJob)
// Block main thread until all jobs are done.
results := pool.StopWaitXT()
// Grab first result
r := results[0]
// Print it to console
fmt.Printf("Name: %s\nData: %v\nDuration: %v\nError?: %v", r.Name(), r.Data, r.Duration(), r.Error)
/*
Name: Hello world job
Data: Hello, world!
Duration: 3.139µs
Error?: <nil>
*/
```

### Using Context

Works with timeouts, cancelled context, etc..

The point is: you have full control over every job.

```go
Job{
  Name: "Job using context",
  Function: func() Result {
    timeout := 10 * time.Second
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    result, err := LongRunningTaskWithContext(ctx)
    if err != nil {
    	return Result{Error: err}
    }
    return Result{Data: result}
  },
}
```

### Using Retry

You can use something like [backoff](https://github.com/cenkalti/backoff) for this (just an example).

The point is: you have full control over every job.

```go
Job{
  Name: "Job using retry",
  Function: func() Result {
    work := func() (string, error) {
      if /* Some result is an error */ {
        return "", theError
      }
      return "IT SUCCEEDED", nil
    }
    expBackoff := backoff.WithBackOff(backoff.NewExponentialBackOff())
    result, err := backoff.Retry(ctx, work, expBackoff)
    if err != nil {
    	return Result{Error: err}
    }
    return Result{Data: result}
  },
}
```
