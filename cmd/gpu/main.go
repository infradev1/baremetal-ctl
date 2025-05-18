package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"baremetal-ctl/cmd/gpu/service"

	"golang.org/x/sync/errgroup"
)

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

	spec := service.WorkerPoolSpec{
		WorkerCount: workerCount,
		BufferSize:  bufferSize,
		RetryCount:  retryCount,
		Group:       g,
	}
	wp := service.NewWorkerPool(spec, &service.RPCSimulator{})
	nodeCount := 10
	results := make(chan *service.GPUHealthResult, nodeCount)

	// producer (simulate)
	for i := range nodeCount {
		job := service.Job{
			Context: ctx,
			CRD: service.GPUHealthCheck{
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
