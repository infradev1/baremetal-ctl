package main

import (
	"context"
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

// the provisioning process for a GPU includes applying this CRD
type Job[T GPUHealthCheck] struct {
	Context context.Context // timeout
	CRD     T
}

type Worker struct {
	Queue chan Job[GPUHealthCheck]
}

type WorkerPoolSpec struct {
	WorkerCount int
	BufferSize  int
	RetryCount  int
	*errgroup.Group
}

type WorkerPool struct {
	Workers []Worker
}

func (wp *WorkerPool) SubmitJob(job Job[GPUHealthCheck]) {
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
	workers := make([]Worker, 0, spec.WorkerCount)

	for range spec.WorkerCount {
		worker := Worker{
			Queue: make(chan Job[GPUHealthCheck], spec.BufferSize),
		}

		spec.Go(func() error {
			// process job queue -> concurrent reconciliation (simulate)
			for job := range worker.Queue { // must be closed by producer(s)
				ctx, cancel := context.WithTimeout(job.Context, 5*time.Second)
				retries := 0

				// retry on failure
				for retries < spec.RetryCount {
					// pass context with timeout to rpc call(s)
					status, err := SimulateRPCCall(ctx, job.CRD.NodeId)
					if err != nil {
						log.Println(err) // export to Prometheus in prod
						retries++
						continue
					}
					log.Println(status)
					break
				}

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

	// producer (mock)
	for i := range 10 {
		job := Job[GPUHealthCheck]{
			Context: ctx,
			CRD: GPUHealthCheck{
				NodeId: fmt.Sprintf("gpu-%d", i*100),
				AZ:     "A",
				Region: "us-east-2",
				Status: "NodeReady",
			},
		}
		wp.SubmitJob(job)
	}
	// close channels since producer is done sending
	wp.Close()

	// consumer (reconciliation) -> wait for all CRD instances to be reconciled
	if err := g.Wait(); err != nil {
		// export Prometheus error metric
		log.Println(err)
	}

}

/*
Task: Write a CLI tool to parallelize GPU health checks across 1000s of nodes using gRPC/DCGM, with:

A worker pool (limit concurrency).
Timeout per node (5s).
Retries for transient failures (max 3 attempts).
Aggregate results (JSON/CSV output).
*/
