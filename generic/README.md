<h1 align="center">workerpoolxt</h1>
<p align="center">
  Worker pool library that extends <a href="https://github.com/gammazero/workerpool">https://github.com/gammazero/workerpool</a>
</p>
<p align="center"><code>go get github.com/matthewoestreich/workerpoolxt/generic</code></p>
<p align="center"><a href="https://pkg.go.dev/github.com/matthewoestreich/workerpoolxt/generic" target="_blank" rel="noopener noreferrer">pkg.go.dev</a></p>

---

## Synopsis

Use `workerpoolxt` with generics. As far as functionality, everything else remains the same. See the README at the root of the repo for extended documentation.

## Import

```go
import wpxt "github.com/matthewoestreich/workerpoolxt/generic"
```

## Example

High level example showing how to use with generics.

[Playground](https://go.dev/play/p/XuaSdqYxWqA)

```go
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
  Web     *WebResult
  Some    *SomeResult
  String  string
}

// Use "container" type as generic
pool := wpxt.New[Output](5)

// Or if you have an existing |*workerpool.WorkerPool| instance
pool := wpxt.WithWorkerPool[Output](existingWorkerPool)

// Create jobs

helloWorldJob := &wpxt.Job[Output]{
  Name: "Hello world job",
  // Function signature must be |func() (T, error)|
  Function: func() (Output, error) {
    var pretendError error = nil
    if pretendError != nil {
      // To demonstrate how you would return an error
      return Output{}, pretendError
    }
    return Output{String: "Hello, world!"}, nil
  },
}

someJob := &wpxt.Job[Output]{
  Name: "Some job",
  Function: func() (Output, error) {
    someResult := &SomeResult{
      Success: true,
      Message: "we did the thing!",
    }
    return Output{Some: someResult}, nil
  },
}

webJob := &wpxt.Job[Output]{
  Name: "Web job",
  Function: func() (Output, error) {
    webResult := &WebResult{Response: "that thing, we did it!"}
    return Output{Web: webResult}, nil
  },
}

// Collect jobs
jobs := []*wpxt.Job[Output]{
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
sameResults := pool.Results()

// Now you can access results in a type-safe manner
for _, result := range results {
  if result.Data.Some != nil {
    // Most likely a job that returned 'SomeJob' type.
    isSuccess := result.Data.Some.Success
    message := result.Data.Some.Message
    jobName := result.Name
    jobDuration := result.Duration
    jobError := result.Error
    fmt.Println(jobName, jobDuration, jobError, isSuccess, message)
  }
  if result.Data.Web != nil {
    // Handle 'WebResult' job
  }
  if result.Data.String != "" {
    // Handle basic string result...
  }
}
```