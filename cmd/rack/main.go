package main

import (
	"baremetal-ctl/cmd/rack/internal"
	"context"
	"encoding/json"
	"flag"
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
	totalPower := &internal.RackPower{}
	results := make(chan internal.Result)

	// simulate input (list of nodes in a rack)
	// TODO: NodePool struct
	nodes, err := internal.CreateNodes(ctx, wg, totalPower, results, nodeCount, defaultNodePower, nodeCapacity, rackPowerBudget, maxConcurrency, bufferSize)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		defer close(results)
		// simulate job submission
		if err := internal.SubmitJobs(nodes, wg, jobCount, jobPower, rackPowerBudget, totalPower); err != nil {
			log.Fatal(err)
		}
		wg.Wait()
		for _, node := range nodes {
			node.Pool.Close()
		}
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
