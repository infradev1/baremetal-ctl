package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type GPUFirmwareRequest struct {
	NodeId    string
	NodePool  string // extensible by adding additional "filter" fields
	ClusterId string // multi-cluster globally distributed fleet
}

type GPUFirmwareResponse struct {
	NodeId  string `json:"nodeId"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

type GPUFirmwareChecker interface {
	GetFirmwareVersion(ctx context.Context, req *GPUFirmwareRequest) (*GPUFirmwareResponse, error) // signature more closely matches a real gRPC client
}

type FirmwareService struct{}

func (fs *FirmwareService) GetFirmwareVersion(ctx context.Context, req *GPUFirmwareRequest) (*GPUFirmwareResponse, error) {
	select {
	case <-ctx.Done():
		return &GPUFirmwareResponse{req.NodeId, "", "request cancelled before verification"}, ctx.Err()
	default:
		return &GPUFirmwareResponse{
			NodeId:  req.NodeId,
			Version: "v1.0.0",
			Status:  "firmware version obtained successfully",
		}, nil
	}
}

type Job struct {
	Id      int
	Context context.Context // with timeout
	Request *GPUFirmwareRequest
	*sync.WaitGroup
}

type Worker struct {
	Id    int
	Queue chan *Job
}

type WorkerPool struct {
	Workers              []*Worker
	LastWorkerSubmission int
}

func (wp *WorkerPool) Close() {
	for _, w := range wp.Workers {
		close(w.Queue)
	}
}

func (wp *WorkerPool) SubmitJob(job *Job) {
	// submit to next worker (round robin load balancing)
	if wp.LastWorkerSubmission == len(wp.Workers) {
		wp.LastWorkerSubmission = 0
	}
	wp.Workers[wp.LastWorkerSubmission].Queue <- job
	wp.LastWorkerSubmission++
}

type WorkerPoolSpec struct {
	WorkerCount int
	BufferSize  int
	RetryCount  int
	Service     GPUFirmwareChecker
	Results     chan *GPUFirmwareResponse
	Errors      chan error
}

func NewWorkerPool(spec *WorkerPoolSpec) *WorkerPool {
	workers := make([]*Worker, 0, spec.WorkerCount)

	for i := range spec.WorkerCount {
		worker := &Worker{
			Id:    i,
			Queue: make(chan *Job, spec.BufferSize),
		}
		workers = append(workers, worker)

		go func() {
			for job := range worker.Queue {
				ctx, cancel := context.WithTimeout(job.Context, 5*time.Second)
				retries := 0
				var lastError error
				cont := true

				log.Printf("WORKER ID: %d\n", worker.Id)

				for retries <= spec.RetryCount && cont {
					select {
					case <-ctx.Done():
						spec.Errors <- ctx.Err()
						cont = false
					default:
						resp, err := spec.Service.GetFirmwareVersion(ctx, job.Request)
						if err != nil {
							lastError = err
							retries++
							time.Sleep(1 * time.Second) // TODO: exponential backoff with jitter
							continue
						}
						spec.Results <- resp
						cont = false
					}
				}
				if retries == 3 {
					spec.Errors <- lastError
				}
				cancel()
				job.Done()
			}
		}()
	}

	return &WorkerPool{
		Workers: workers,
	}
}

const total = 5

func main() {
	var workerCount int
	flag.IntVar(&workerCount, "workerCount", 10, "Goroutine concurrency")

	var bufferSize int
	flag.IntVar(&bufferSize, "bufferSize", 100, "Goroutine channel size")

	var retries int
	flag.IntVar(&retries, "retries", 3, "gRPC transient error retries")

	var format string
	flag.StringVar(&format, "format", "json", "results output format in json or text (default: json)")

	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT)
	defer cancel()

	results := make(chan *GPUFirmwareResponse, total)
	errors := make(chan error, total)

	spec := &WorkerPoolSpec{
		WorkerCount: workerCount,
		BufferSize:  bufferSize,
		RetryCount:  retries,
		Service:     &FirmwareService{},
		Results:     results,
		Errors:      errors,
	}
	wp := NewWorkerPool(spec)
	wg := &sync.WaitGroup{}

	go func() {
		defer wp.Close()
		defer close(results)
		defer close(errors)

		for i := range total {
			wg.Add(1)
			wp.SubmitJob(&Job{
				Id:      i,
				Context: ctx,
				Request: &GPUFirmwareRequest{
					NodeId:    fmt.Sprintf("gpu-%d", i),
					NodePool:  "default", // placeholder
					ClusterId: "admin",   // placeholder
				},
				WaitGroup: wg,
			})
		}
		wg.Wait()
	}()

	// receive responses
	for result := range results {
		if format == "json" {
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				log.Println(err)
				continue
			}
			fmt.Println(string(data))
		} else {
			log.Println(result)
		}
	}

	// receive errors
	for err := range errors {
		log.Println(err)
	}
}

/*
Build a CLI tool to perform concurrent GPU firmware version checks across a distributed node fleet via gRPC.
Each node exposes a gRPC endpoint that responds with the current firmware version. Your tool must:

Accept a list of node hostnames (you can simulate them as strings: node-1, node-2, etc.).
Use a bounded worker pool to parallelize health checks across nodes (default: 10 workers).
Set a timeout per node check (5 seconds).
Retry transient failures (e.g., timeouts or temporary network errors) up to 3 times.
Aggregate results (node ID, firmware version or error) and print as either:
--format=json
--format=csv

The actual gRPC client can be simulated by a FirmwareChecker interface.
Simulate random failures in your implementation, and handle them robustly.
*/
