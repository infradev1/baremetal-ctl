package internal

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// replace parameter list with WorkerPoolSpec for maintainability and extensibility
func NewWorkerPool(ctx context.Context, results chan<- Result, wg *sync.WaitGroup, workerCount, bufferSize int) *WorkerPool {
	workers := make([]*Worker, 0, workerCount)
	for i := range workerCount {
		worker := &Worker{
			Id:    fmt.Sprintf("worker-%d", i),
			Queue: make(chan *Job),
		}
		workers = append(workers, worker)

		go func() {
			for job := range worker.Queue {
				// capture results channel
				// set up context with deadline for the job (simulate gRPC call)
				// due to time constraints, simply echo job message
				results <- Result{fmt.Sprintf("%s from (%s, %s)", job.Message, job.NodeId, worker.Id)}
				wg.Done()
			}
		}()
	}
	return &WorkerPool{Workers: workers}
}

func SubmitJobs(nodes []*Node, wg *sync.WaitGroup, jobCount, jobPower, rackPowerBudget int, totalPower *int) error {
	lastSubmitted := 0
	// simulate job submission
	for i := range jobCount {
		// ensure we do not exceed total rack power (block until jobs finish rather than log fatal, if time allows)
		if *totalPower > rackPowerBudget {
			return errors.New("exceeded rack power budget with job submission")
		}

		// submit round-robin
		if lastSubmitted >= len(nodes) {
			lastSubmitted = 0
		}
		wg.Add(1)
		nodes[lastSubmitted].SubmitJob(&Job{
			Message: fmt.Sprintf("message number %d", i),
			NodeId:  nodes[lastSubmitted].Id,
			Power:   jobPower,
		}) // for simplicity
		*totalPower += jobPower
		lastSubmitted++
	}
	return nil
}
