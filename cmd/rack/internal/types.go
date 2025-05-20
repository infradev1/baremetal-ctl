package internal

import (
	"log"
	"sync"
)

type RackPower struct {
	Watts int
	sync.Mutex
}

func (rp *RackPower) Add(value int) {
	rp.Lock()
	defer rp.Unlock()
	rp.Watts += value
}

func (rp *RackPower) Get() int {
	rp.Lock()
	defer rp.Unlock()
	return rp.Watts
}

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

type Node struct {
	Id                  string
	PowerUsage          int         // Watts
	Capacity            int         // Watts
	Assigned            int         // initially zero
	Pool                *WorkerPool // simulate daemonset (1 replica per node in the rack)
	LastSubmittedWorker int
}

type Job struct {
	Message string
	NodeId  string
	Power   int
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
	n.PowerUsage += job.Power
	n.LastSubmittedWorker++
}
