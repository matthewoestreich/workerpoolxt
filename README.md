<h1 align="center">workerpoolxt</h1>
<p align="center">
  Worker pool library that extends <a href="https://github.com/gammazero/workerpool">https://github.com/gammazero/workerpool</a>
</p>
<p align="center"><code>go get github.com/matthewoestreich/workerpoolxt</code></p>
<p align="center"><a href="https://pkg.go.dev/github.com/matthewoestreich/workerpoolxt" target="_blank" rel="noopener noreferrer">pkg.go.dev</a></p>

---

## Synopsis

We wrap the func(s) that get passed to `workerpool`, which we call "jobs". Job results will be returned to you after all jobs have completed.

You have the ability to give each job a name. You can access job results, job runtime duration, or any job errors within job results. In `gammazero/workerpool` this is not possible - you do not have the ability to get any sort of result or error data from a "job".

**You still retain access to all `gammazero/workerpool` members. See [`Native Member Access`](#native-member-access) for more.**

## Generics

If you want to use this package with type-safe generics, [please see here](/generic).

## Import

```go
import wpxt "github.com/matthewoestreich/workerpoolxt"
```

## Examples

### Hello, world!

[Playground](https://go.dev/play/p/4oKDsprC8dC)

```go
numWorkers := 5
pool := wpxt.New(numWorkers)

helloWorldJob := &wpxt.Job{
  Name: "Hello world job",
  // Function signature must be |func() (any, error)|
  Function: func() (any, error) {
    pretendError := nil
    if pretendError != nil {
      // To demonstrate how you would return an error
      return nil, theError
    }
    return "Hello, world!", nil
  },
}

// Submit job
pool.SubmitXT(helloWorldJob)

// Block main thread until all jobs are done.
results := pool.StopWaitXT()

// Results also accessable via
sameResults := pool.Results()

// Grab first result
r := results[0]

// Print it to console
fmt.Printf(
  "Name: %s\nData: %v\nDuration: %v\nError?: %v", 
  r.Name, r.Data, r.Duration, r.Error,
)
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
&wpxt.Job{
  Name: "Job using context",
  Function: func() (any, error) {
    timeout := 10 * time.Second
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    result, err := LongRunningTaskWithContext(ctx)
    if err != nil {
    	return nil, err
    }
    return result, nil
  },
}
```

### Using Retry

You can use something like [backoff](https://github.com/cenkalti/backoff) for this (just an example).

The point is: you have full control over every job.

```go
&wpxt.Job{
  Name: "Job using retry",
  Function: func() (any, error) {
    work := func() (string, error) {
      if /* Some result is an error */ {
        return "", theError
      }
      return "IT SUCCEEDED", nil
    }
    expBackoff := backoff.WithBackOff(backoff.NewExponentialBackOff())
    result, err := backoff.Retry(ctx, work, expBackoff)
    if err != nil {
    	return nil, err
    }
    return result, nil
  },
}
```

# Native Member Access

Using native `gammazero/workerpool` members with `workerpoolxt`.

## StopWait

If you would like to use the native `.StopWait()` instead of `allResults := .StopWaitXT()`, here is how you can still gain access to job results.

```go
wp := wpxt.New(4)

//
// Submitting a bunch of jobs here
//

wp.StopWait()

allResults := wp.Results()
// Do something with |allResults|
```