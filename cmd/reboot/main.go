package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Node struct {
	Id     int    `json:"id"`
	AZ     string `json:"az"`
	Region string `json:"region"`
}

type RebootResult struct {
	NodeId int    `json:"nodeId"`
	Status string `json:"status"` // TODO: replace with enum
	Error  string `json:"error,omitempty"`
}

type Rebooter interface {
	Reboot(ctx context.Context, node *Node) *RebootResult
}

// simulate gRPC client
type Client struct{}

func (c *Client) Reboot(ctx context.Context, node *Node) *RebootResult {
	// if time allows, simulate all 3 result modes
	return &RebootResult{
		NodeId: node.Id,
		Status: "success",
		Error:  "",
	}
}

type RebootsInProgress struct {
	AZs map[string]int // count of node reboots in progress per AZ
	sync.Mutex
}

func (r *RebootsInProgress) Get(az string) int {
	r.Lock()
	defer r.Unlock()

	return r.AZs[az]
}

func (r *RebootsInProgress) Add(az string, value int) {
	r.Lock()
	defer r.Unlock()

	r.AZs[az] += value
}

func NewRebootsInProgress() *RebootsInProgress {
	return &RebootsInProgress{
		AZs: make(map[string]int),
	}
}

type Worker struct {
	Id    int // mostly for debugging
	Queue chan *Node
}

type WorkerPool struct {
	Workers       []*Worker
	LastSubmitted int
}

func (wp *WorkerPool) Submit(node *Node) {
	// round-robin load balancing
	if wp.LastSubmitted == len(wp.Workers) {
		wp.LastSubmitted = 0
	}
	wp.Workers[wp.LastSubmitted].Queue <- node
}

func (wp *WorkerPool) Close() {
	for _, worker := range wp.Workers {
		close(worker.Queue)
	}
}

type WorkerPoolSpec struct {
	WorkerCount int // bounded worker pool
	BufferSize  int // // buffered channels to limit backpressure (although the limit is 5 globally, so 5 workers and unbuffered channel may be enough)
	Retries     int
	Results     chan<- *RebootResult
	Activity    *RebootsInProgress
	Service     Rebooter
	*sync.WaitGroup
	context.Context
}

func NewWorkerPool(spec *WorkerPoolSpec) *WorkerPool {
	workers := make([]*Worker, 0, spec.WorkerCount)

	for i := range spec.WorkerCount {
		worker := &Worker{
			Id:    i,
			Queue: make(chan *Node),
		}

		go func() {
			for node := range worker.Queue {
				// consider redesign to have 1 worker per AZ
				if inProgress := spec.Activity.Get(node.AZ); inProgress < 2 {
					spec.WaitGroup.Add(1)
					spec.Activity.Add(node.AZ, 1)
					ctx, cancel := context.WithTimeout(spec.Context, 10*time.Second)
					done := false

					for attempt := 0; attempt <= spec.Retries && !done; attempt++ {
						select {
						case <-ctx.Done():
							spec.Results <- &RebootResult{
								NodeId: node.Id,
								Status: "timeout",
								Error:  ctx.Err().Error(),
							}
							done = true
						default:
							result := spec.Service.Reboot(ctx, node)
							if result.Status == "success" {
								spec.Results <- result
								done = true
							} else {
								time.Sleep(1 * time.Second) // TODO: exponential backoff with jitter
								continue
							}

						}
					}
					cancel()
					spec.Activity.Add(node.AZ, -1)
					spec.WaitGroup.Done()
				} else {
					// re-enqueue it and continue
					worker.Queue <- node
				}
			}
		}()

		workers = append(workers, worker)
	}

	return &WorkerPool{
		Workers: workers,
	}
}

// Write a Go program that orchestrates safe reboots of a list of nodes.
// Reboot all nodes with:
// Max 5 concurrent reboots globally (done)
// No more than 2 nodes rebooting per AZ at a time (done)
// Each reboot must:
// Timeout after 10 seconds
// Retry up to 2 times on transient failure
// Output final status per node in JSON format
func main() {
	var workerCount int
	flag.IntVar(&workerCount, "workers", 5, "Max concurrent reboots globally (default: 5)")

	var bufferSize int
	flag.IntVar(&bufferSize, "bufferSize", 100, "Worker channel buffer size (default: 5)")

	var retryCount int
	flag.IntVar(&retryCount, "retries", 2, "Max retries per reboot request (default: 2)")

	flag.Parse()

	// use in-memory slice of nodes if provided (or generate a sample)
	// assuming nodes.json provided
	file, err := os.ReadFile("nodes.json")
	if err != nil {
		log.Fatal(err)
	}

	var nodes []Node
	if err := json.Unmarshal(file, &nodes); err != nil {
		log.Fatal(err)
	}

	results := make(chan *RebootResult, len(nodes))
	svc := new(Client)
	wg := &sync.WaitGroup{}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	spec := &WorkerPoolSpec{
		WorkerCount: workerCount,
		BufferSize:  bufferSize,
		Retries:     retryCount,
		Results:     results,
		Activity:    NewRebootsInProgress(),
		Service:     svc,
		WaitGroup:   wg,
		Context:     ctx,
	}
	wp := NewWorkerPool(spec)

	go func() {
		defer wp.Close()
		defer close(results)
		for _, node := range nodes {
			wp.Submit(&node)
		}
		wg.Wait()
	}()

	total := 0

	for result := range results {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Println(err)
		}
		log.Println(string(data))
		total++
	}

	log.Println(total)
}
