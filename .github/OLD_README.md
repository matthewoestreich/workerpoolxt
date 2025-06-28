
<h1 align="center">workerpoolxt<span>
<a href="https://github.com/oze4/workerpoolxt/blob/master/LICENSE">
<img alt="GitHub" src="https://img.shields.io/github/license/oze4/workerpoolxt?style=social">
</a></span></h1>


<p align="center">
<a href="https://github.com/oze4/workerpoolxt/actions?query=workflow%3ABuild">
<img alt="GitHub Workflow Status" src="https://img.shields.io/github/workflow/status/oze4/workerpoolxt/Build?logo=github&style=flat-square">
</a>
<a href="https://coveralls.io/github/oze4/workerpoolxt">
<img alt="Coveralls github" src="https://img.shields.io/coveralls/github/oze4/workerpoolxt?label=coveralls&logo=coveralls&style=flat-square">
  <br />
  <a href="https://app.codacy.com/gh/oze4/workerpoolxt/dashboard">
<img alt="Codacy grade" src="https://img.shields.io/codacy/grade/782dd1e1d8844b129f4de4df7984b537?logo=codacy&style=flat-square">
</a>
  <a href="https://pkg.go.dev/github.com/oze4/workerpoolxt">
    <img title="PkgGoDev" src="https://img.shields.io/badge/%20%20%20-reference-29beb0?style-for-the-badge&logo=go&labelColor=gray&logoColor=white&message=reference&style=flat-square" >
  </a>
    <a href="https://goreportcard.com/report/github.com/oze4/workerpoolxt">
    <img title="Go Report Card" src="https://goreportcard.com/badge/github.com/oze4/workerpoolxt?style=flat-square" >
  </a>
</p>
<p align="center">
  Worker pool library that extends <a href="https://github.com/gammazero/workerpool">https://github.com/gammazero/workerpool</a>
</p>

# Note

***If you are using context, please use with caution in production!! [See here for more info](https://github.com/oze4/workerpoolxt/issues/12#issuecomment-716794599)***

 - [Go Playground](https://play.golang.org/p/pYGz7Ryj8VQ) which shows the area of concern
 
 - Wonderful explanation from the author of `workerpool`:
 
>Once a goroutine is started, there no way to kill it unless there is something inside that goroutine that is looking for a signal, waiting on a context or channel, etc. In the case of workerpool, and workerpoolxt, the pool is providing the goroutine and calling someone else's function in that goroutine. There is no telling if that function will ever return and allow your goroutine to finish or run another task. The only thing that workerpoolxt can do is to run the task function in another goroutine and wait for the task to finish or for a timer to expire or some other signal to give up waiting (such as context being canceled). When this signal to give up happens, the worker goroutine can return and report an error, but the task goroutine is still running somewhere in the background and may or may not ever finish. All you have done is given up waiting for it to do so.

>\- @gammazero github.com/gammazero

## Synopsis

  - Allows you to retain access to underlying `*WorkerPool` object as if you imported `workerpool` directly
  - [Hello, world!](#hello-world)
  - [How we extend `workerpool`](#how-we-extend-workerpool)
    - [Results](#results)
      - Job results are captured so you can work with them later
      - [How to handle errors?](#error-handling)
    - [Context](#context)
      - Supply your own context
      - "Default" context required when calling `workerpoolxt.New(...)`
      - You can override the [default context](#default-context) on a [per job basis](#per-job-context)
      - [This allows you to do things like custom job timeouts](#timeouts)
    - [Retry](#retry)
      - `int` that defines N number of retries
      - Can only supply retry on a per job basis
    - [Options](#options)
      - Options are optional
      - Provide either [global/default options](#default-options) or [per job options](#per-job-options)
      - Options are nothing more than `map[string]interface{}` so that you may supply anything you wish
      - Job options override default options, **_we do NOT merge options_**
    - Runtime duration
      - Access a job's runtime duration via it's result
      - e.g. `howLongItTook := someResultFromSomeJob.Duration time.Duration`

---

## Hello World

- Obligatory "*as simple as it gets*" example

```golang
package main

import (
    "context"
    "fmt"
    wpxt "github.com/oze4/workerpoolxt"
)

func main() {
    ctx := context.Background()
    numWorkers := 10

    wp := wpxt.New(ctx, numWorkers)

    wp.SubmitXT(wpxt.Job{
        Name: "My first job",
        Task: func(o wpxt.Options) wpxt.Result {
            return wpxt.Result{Data: "Hello, world!"}
        },
    })

    jobResults := wp.StopWaitXT()

    for _, jobresult := range jobResults {
        fmt.Println(jobresult)
    }
}
```

---

# How we extend `workerpool`

## Results

```golang
// ...
// ... pretend we submitted jobs here
// ...

results := wp.StopWaitXT() // -> []wpxt.Result

for _, result := range results {
    // If job failed, `result.Error != nil`
}
```

### Error Handling

- What if I encounter an error in one of my jobs?
- How can I handle or check for errors/timeout?

#### Return Error From `Job`

```golang
// Just set the `Error` field on the `wpxt.Result` you return
wp.SubmitXT(wpxt.Job{
    Name: "How to handle errors",
    Task: func(o wpxt.Options) wpxt.Result {
        // Pretend we got an error doing something
        if theError != nil {
            return wpxt.Result{Error: theError}
        }
    },
})
```

#### Check For Errors In `Result`

```golang
// ... pretend we submitted a bunch of jobs
//
// StopWaitXT() returns []wpxt.Result
// Each result has an `Error` field
// Whether a timeout, or an error you set
// Check for it like
if someResultFromSomeJob.Error != nil {
    // ....
}
```

## Context

 - Required default context when creating new `workerpoolxt`
 - You can override default context per job

### Default Context

```golang
myctx := context.Background() // Any `context.Context`
numWorkers := 10
wp := wpxt.New(myctx, numWorkers)
```

### Per Job Context

#### Timeouts

```golang
defaultCtx := context.Background()
numWorkers := 10
wp := wpxt.New(defaultCtx, numWorkers)
timeout := time.Duration(time.Millisecond)

myCtx, done := context.WithTimeout(context.Background(), timeout)
defer done()

wp.SubmitXT(wpxt.Job{
    Name: "my ctx job",
    Context: myCtx,
    Task: func(o wpxt.Options) wpxt.Result {
        // Simulate long running task
        time.Sleep(time.Second*10) 
        return wpxt.Result{Data: "I could be anything"}
    },
})
// > `Result.Error` will be `context.DeadlineExceeded`
```

## Retry

- Optional
- Seamlessly retry failed jobs

```golang
wp.SubmitXT(wpxt.Job{
    // This job is configured to fail immediately, 
    // therefore it will retry 5 times
    // (as long as we have not exceeded our job timeout)
    timeoutctx, _ := context.WithTimeout(context.Background(), time.Duration(time.Millisecond*500))
    Retry: 5,
    // ^^^^^^
    Name: "I will retry 5 times",
    // Set timeout field on job
    Context: timeoutctx,
    Task: func(o wpxt.Options) wpxt.Result {
        return wpxt.Result{Error: errors.New("some_err")}
    },
})
```

## Options

- Help make jobs flexible

### Default Options

```golang
myopts := map[string]interface{}{
    "myclient": &http.Client{},
}

wp := wpxt.New(context.Background(), 10)
wp.WithOptions(myopts)

wp.SubmitXT(wpxt.Job{
    Name: "myjob",
    Task: func(o wpxt.Options) wpxt.Result {
        // access options here
        client := o["myclient"]
    },
})
```

### Per Job Options

```golang
myhttpclient := &http.Client{}
myk8sclient := kubernetes.Clientset{}

// This Job Only Needs an HTTP Client
wp.SubmitXT(wpxt.Job{
    Name: "This Job Only Needs an HTTP Client",
    Options: map[string]interface{}{
        "http": myhttpclient,
    },
    Task: func(o wpxt.Options) wpxt.Result {
        // access options here
        httpclient := o["http"]
        // ... do work with `httpclient`
    },
})

// This Job Only Needs Kubernetes Clientset
wp.SubmitXT(wpxt.Job{
    Name: "This Job Only Needs Kubernetes Clientset",
    Options: map[string]interface{}{
        "kube": myk8sclient,
    },
    Task: func(o wpxt.Options) wpxt.Result {
        // access options here
        kubernetesclient := o["kube"]
        // ... do work with `kubernetesclient`
    },
})
```
