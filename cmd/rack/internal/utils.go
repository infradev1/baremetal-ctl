package internal

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// replace parameter list with WorkerPoolSpec for maintainability and extensibility
func NewWorkerPool(ctx context.Context, results chan<- Result, wg *sync.WaitGroup, workerCount, bufferSize int, totalPower *RackPower) *WorkerPool {
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
				// decrement total rack power
				totalPower.Add(-job.Power)
			}
		}()
	}
	return &WorkerPool{Workers: workers}
}

// TODO: Pass NodeSpec and WorkerPoolSpec structs
func CreateNodes(ctx context.Context, wg *sync.WaitGroup, totalPower *RackPower, results chan<- Result,
	nodeCount, defaultNodePower, nodeCapacity, rackPowerBudget, maxConcurrency, bufferSize int,
) ([]*Node, error) {
	nodes := make([]*Node, 0, nodeCount)
	for i := range nodeCount {
		if totalPower.Get() > rackPowerBudget {
			return nil, errors.New("exceeded rack power budget")
		}
		node := &Node{
			Id:         fmt.Sprintf("node-%d", i),
			PowerUsage: defaultNodePower,
			Capacity:   nodeCapacity, // could also be a CLI flag
			Assigned:   0,
			Pool:       NewWorkerPool(ctx, results, wg, maxConcurrency, bufferSize, totalPower),
		}
		nodes = append(nodes, node)
		totalPower.Add(defaultNodePower)
	}
	return nodes, nil
}

func SubmitJobs(nodes []*Node, wg *sync.WaitGroup, jobCount, jobPower, rackPowerBudget int, totalPower *RackPower) error {
	lastSubmitted := 0
	// simulate job submission
	for i := range jobCount {
		// ensure we do not exceed total rack power (block until jobs finish rather than log fatal, if time allows)
		currentTotal := totalPower.Get()
		if currentTotal+jobPower > rackPowerBudget {
			return errors.New("exceeded rack power budget with job submission")
		}
		// submit round-robin
		if lastSubmitted >= len(nodes) {
			lastSubmitted = 0
		}
		nodes[lastSubmitted].SubmitJob(&Job{
			Message: fmt.Sprintf("message number %d", i),
			NodeId:  nodes[lastSubmitted].Id,
			Power:   jobPower,
		}) // for simplicity
		totalPower.Add(jobPower)
		lastSubmitted++
		wg.Add(1)
	}
	return nil
}
