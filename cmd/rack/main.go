package main

import (
	"baremetal-ctl/cmd/rack/internal"
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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	wg := &sync.WaitGroup{}

	totalPower := 0
	// simulate input (list of nodes in a rack)
	nodes := make([]*internal.Node, 0, nodeCount)
	results := make(chan internal.Result)

	for i := range nodeCount {
		if totalPower > rackPowerBudget {
			log.Fatal("exceeded rack power budget")
		}
		node := &internal.Node{
			Id:         fmt.Sprintf("node-%d", i),
			PowerUsage: defaultNodePower,
			Capacity:   nodeCapacity, // could also be a CLI flag
			Assigned:   0,
			Pool:       internal.NewWorkerPool(ctx, results, wg, maxConcurrency, bufferSize),
		}
		nodes = append(nodes, node)
		totalPower += defaultNodePower
	}

	go func() {
		defer close(results)
		// simulate job submission
		if err := internal.SubmitJobs(nodes, wg, jobCount, jobPower, rackPowerBudget, &totalPower); err != nil {
			log.Fatal(err)
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
