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
)

const (
	nodeCount        = 5
	nodeCapacity     = 1000 // Watts
	rackPowerBudget  = 8000 // Watts
	jobPower         = 100  //assume each job draws 100W
	jobCount         = 10   // 100 jobs to be processed by the nodes in the rack
	defaultNodePower = 200
)

type Result struct {
	Message string `json:"message"`
}

type Worker struct {
	Id    string
	Queue chan *Job
}

type WorkerPool struct {
	Workers []*Worker
}

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
				results <- Result{job.Message}
				wg.Done()
			}
		}()
	}
	return &WorkerPool{Workers: workers}
}

type Node struct {
	Id                  string
	PowerUsage          int         // Watts
	Capacity            int         // Watts
	Assigned            int         // initially zero
	Pool                *WorkerPool // simulate daemonset (1 replica per node in the rack)
	LastSubmittedWorker int
}

func (n *Node) SubmitJob(job *Job) {
	if n.PowerUsage > n.Capacity {
		log.Fatal("exceeded node power capacity") // TODO: graceful handling, if time allows
	}
	// round-robin load balancing within the node as well
	if n.LastSubmittedWorker >= len(n.Pool.Workers) {
		n.LastSubmittedWorker = 0
	}
	n.Pool.Workers[n.LastSubmittedWorker].Queue <- job
	n.PowerUsage += jobPower
	n.LastSubmittedWorker++
}

// replace parameter list with NodeSpec for maintainability and extensibility
func NewNode(nodeId string, ctx context.Context, results chan Result, wg *sync.WaitGroup, workerCount, bufferSize int) *Node {
	return &Node{
		Id:         nodeId,
		PowerUsage: defaultNodePower,
		Capacity:   nodeCapacity, // could also be a CLI flag
		Assigned:   0,
		Pool:       NewWorkerPool(ctx, results, wg, workerCount, bufferSize),
	}
}

type Job struct {
	Message string
}

// Build a Go CLI tool that assigns jobs to nodes in a rack without exceeding the rack-wide power budget.
// Assign jobs to nodes such that:
// No node exceeds its Capacity
// The total power draw after assignment does not exceed the rack budget
// Output the final assignment per node in JSON format.
func main() {
	var maxConcurrency int
	flag.IntVar(&maxConcurrency, "maxConcurrency", 5, "Maximum concurrent goroutines per node (default: 5)")

	var bufferSize int
	flag.IntVar(&bufferSize, "bufferSize", 100, "Goroutine channel buffer size (default: 100)")

	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT)
	defer cancel()

	wg := &sync.WaitGroup{}

	totalPower := 0
	// simulate input (list of nodes in a rack)
	nodes := make([]*Node, 0, nodeCount)
	results := make(chan Result)

	for i := range nodeCount {
		if totalPower > rackPowerBudget {
			log.Fatal("exceeded rack power budget")
		}
		nodes = append(nodes, NewNode(
			fmt.Sprintf("node-%d", i),
			ctx,
			results,
			wg,
			maxConcurrency,
			bufferSize,
		))
		totalPower += defaultNodePower
	}

	go func() {
		defer close(results)
		lastSubmitted := 0
		// simulate job submission
		for i := range jobCount {
			// ensure we do not exceed total rack power (block until jobs finish rather than log fatal, if time allows)
			if totalPower > rackPowerBudget {
				log.Fatal("exceeded rack power budget with job submission")
			}

			// submit round-robin
			if lastSubmitted >= len(nodes) {
				lastSubmitted = 0
			}
			wg.Add(1)
			nodes[lastSubmitted].SubmitJob(&Job{fmt.Sprintf("message number %d", i)}) // for simplicity
			totalPower += jobPower
			lastSubmitted++
		}
		wg.Wait()
	}()

	// log results from channel in JSON format, if time allows
	for result := range results {
		bytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Println(err) // for simplicity and time
		}
		log.Println(string(bytes))
	}
}
