package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
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

type GPUService interface {
	CheckHealth(ctx context.Context, requestID, nodeId string) (string, error)
}

type RPCSimulator struct{}

func (rpc *RPCSimulator) CheckHealth(ctx context.Context, requestID, nodeId string) (string, error) {
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

// the provisioning process for a GPU includes applying this CRD
type Job struct {
	RequestID string          // Unique ID for idempotency (e.g., hash of NodeId+Timestamp)
	Context   context.Context // timeout
	CRD       GPUHealthCheck
	Results   chan<- *GPUHealthResult
}

type Worker struct {
	Id    int
	Queue chan Job
}

type WorkerPoolSpec struct {
	WorkerCount int
	BufferSize  int
	RetryCount  int
	*rate.Limiter
	*errgroup.Group
}

type WorkerPool struct {
	Workers []*Worker
}

func (wp *WorkerPool) Close() {
	for _, w := range wp.Workers {
		close(w.Queue)
	}
}

func (wp *WorkerPool) SubmitJob(job Job) {
	if !isDuplicate(job.RequestID) { // Check cache/db
		// in production, use round-robin load balancing or partition key hashing function like Kafka
		i := rand.Intn(len(wp.Workers))
		wp.Workers[i].Queue <- job
	}
}

var processedIDs sync.Map // Thread-safe

func isDuplicate(id string) bool {
	_, loaded := processedIDs.LoadOrStore(id, true)
	return loaded
}

func NewWorkerPool(spec *WorkerPoolSpec, svc GPUService) *WorkerPool {
	workers := make([]*Worker, 0, spec.WorkerCount)

	for i := range spec.WorkerCount {
		worker := &Worker{
			Id:    i,
			Queue: make(chan Job, spec.BufferSize),
		}

		spec.Go(func() error {
			// process job queue -> concurrent reconciliation (simulate)
			for job := range worker.Queue { // must be closed by producer(s)
				if err := spec.Limiter.Wait(job.Context); err != nil {
					return fmt.Errorf("rate limit failed: %w", err) // Respects context cancellation
				}

				ctx, cancel := context.WithTimeout(job.Context, 5*time.Second)
				retries := 0
				var result *GPUHealthResult
				var lastError error

				log.Printf("WORKER ID: %d\n", worker.Id)

				// retry on failure
				for retries < spec.RetryCount {
					// pass context with timeout to rpc call(s)
					requestID := uuid.New().String()
					if _, err := svc.CheckHealth(ctx, requestID, job.CRD.NodeId); err != nil {
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
