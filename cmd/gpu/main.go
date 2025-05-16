package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

// K8s CRD
type GPUHealthCheck struct {
	NodeId string
	AZ     string
	Region string
	Status string
}

type GPUHealthResult struct {
	NodeId  string `json:"node_id"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

// the provisioning process for a GPU includes applying this CRD
type Job struct {
	Context context.Context // timeout
	CRD     GPUHealthCheck
	Results chan<- *GPUHealthResult
}

type Worker struct {
	Id    int
	Queue chan Job
}

type WorkerPoolSpec struct {
	WorkerCount int
	BufferSize  int
	RetryCount  int
	*errgroup.Group
}

type WorkerPool struct {
	Workers []*Worker
}

func (wp *WorkerPool) SubmitJob(job Job) {
	// in production, use round-robin load balancing or partition key hashing function like Kafka
	i := rand.Intn(len(wp.Workers))
	wp.Workers[i].Queue <- job
}

func (wp *WorkerPool) Close() {
	for _, w := range wp.Workers {
		close(w.Queue)
	}
}

func NewWorkerPool(spec WorkerPoolSpec) *WorkerPool {
	workers := make([]*Worker, 0, spec.WorkerCount)

	for i := range spec.WorkerCount {
		worker := &Worker{
			Id:    i,
			Queue: make(chan Job, spec.BufferSize),
		}

		spec.Go(func() error {
			// process job queue -> concurrent reconciliation (simulate)
			for job := range worker.Queue { // must be closed by producer(s)
				ctx, cancel := context.WithTimeout(job.Context, 5*time.Second)
				retries := 0
				var result *GPUHealthResult
				var lastError error

				log.Printf("WORKER ID: %d\n", worker.Id)

				// retry on failure
				for retries < spec.RetryCount {
					// pass context with timeout to rpc call(s)
					if _, err := SimulateRPCCall(ctx, job.CRD.NodeId); err != nil {
						lastError = err // export to Prometheus in prod
						retries++
						time.Sleep(1 * time.Second) // exponential backoff with jitter in prod
						continue
					}
					break
				}

				if retries == spec.RetryCount {
					result = &GPUHealthResult{
						NodeId:  job.CRD.NodeId,
						Healthy: false,
						Error:   lastError.Error(),
					}
				} else {
					result = &GPUHealthResult{
						NodeId:  job.CRD.NodeId,
						Healthy: true,
					}
				}

				job.Results <- result
				cancel()
			}
			return nil
		})

		workers = append(workers, worker)
	}
	return &WorkerPool{workers}
}

func SimulateRPCCall(ctx context.Context, nodeId string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
		// simulate random failure
		if i := rand.Intn(10); i%2 == 0 {
			return "", errors.New("rpc unresponsive")
		}
		return fmt.Sprintf("GPU %s healthy", nodeId), nil
	}
}

// controller watching CRD
func main() {
	var workerCount int
	flag.IntVar(&workerCount, "workerCount", 5, "Number of goroutines in worker pool (default: 5)")

	var bufferSize int
	flag.IntVar(&bufferSize, "bufferSize", 100, "Goroutine channel buffer size (default: 100)")

	var retryCount int
	flag.IntVar(&retryCount, "retryCount", 3, "gRPC API call retry count (default: 3)")

	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workerCount)

	spec := WorkerPoolSpec{workerCount, bufferSize, retryCount, g}
	wp := NewWorkerPool(spec)
	nodeCount := 10
	results := make(chan *GPUHealthResult, nodeCount)

	// producer (mock)
	for i := range nodeCount {
		job := Job{
			Context: ctx,
			CRD: GPUHealthCheck{
				NodeId: fmt.Sprintf("gpu-%d", i*100),
				AZ:     "A",
				Region: "us-east-2",
				Status: "NodeReady",
			},
			Results: results,
		}
		wp.SubmitJob(job)
	}
	// close channels since producer is done sending
	wp.Close()

	// consumer (reconciliation) -> wait for all CRD instances to be reconciled
	go func() {
		if err := g.Wait(); err != nil {
			// export Prometheus error metric
			log.Println(err)
		}
		close(results)
	}()

	// print JSON log
	// created a sorted slice for deterministic output
	for result := range results {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Fatalf("error marshaling result: %v", result)
			continue
		}
		log.Println(string(data))
	}
}

/*
Task: Write a CLI tool to parallelize GPU health checks across 1000s of nodes using gRPC/DCGM, with:

A worker pool (limit concurrency).
Timeout per node (5s).
Retries for transient failures (max 3 attempts).
Aggregate results (JSON/CSV output).
*/
