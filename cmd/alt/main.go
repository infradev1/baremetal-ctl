package main

import (
	"bufio"
	"cmp"
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sync/errgroup"
)

// type aliases for readability
type NodeName = string
type StatusCounts = map[string]int // i.e. "ERROR": 3

type Log struct {
	nodeInfo map[NodeName]StatusCounts
	sync.Mutex
}

type LogSummary struct {
	node  NodeName
	total int
	types StatusCounts
}

func NewLog() *Log {
	return &Log{
		nodeInfo: make(map[NodeName]StatusCounts),
	}
}

func (l *Log) Update(name NodeName, status string) {
	l.Lock()
	defer l.Unlock()

	if _, ok := l.nodeInfo[name]; !ok {
		l.nodeInfo[name] = make(map[string]int)
	}

	l.nodeInfo[name][status]++
}

func writeFile(contents []*LogSummary) error {
	out, err := os.Create("out.txt")
	if err != nil {
		return fmt.Errorf("error creating output file: %w", err)
	}
	defer out.Close()

	w := bufio.NewWriter(out)
	defer w.Flush()

	total := 0
	for _, s := range contents {
		_, err := fmt.Fprintf(w, "%s: %d total (INFO=%d, ERROR=%d, WARN=%d)\n",
			s.node, s.total, s.types["[INFO]"], s.types["[ERROR]"], s.types["[WARN]"],
		)
		if err != nil {
			return fmt.Errorf("error writing output: %w", err)
		}
		total += s.total
	}
	log.Printf("%d total log entries processed", total)

	return nil
}

// gpu-node-17: 5 total (INFO=3, ERROR=1, WARN=1)
// gpu-node-32: 2 total (INFO=1, ERROR=1, WARN=0)
func (l *Log) WriteSummary(sortBy string, outFormat string) error {
	summary := make([]*LogSummary, 0)

	for node, status := range l.nodeInfo {
		total := 0
		for _, count := range status {
			total += count
		}

		summary = append(summary, &LogSummary{
			node:  node,
			total: total,
			types: status,
		})
	}

	slices.SortFunc(summary, func(a, b *LogSummary) int {
		if sortBy == "total" {
			return cmp.Compare(b.total, a.total) // descending order
		}
		return cmp.Compare(a.node, b.node) // descending order
	})

	if outFormat == "file" {
		return writeFile(summary)
	}

	total := 0
	for _, s := range summary {
		log.Printf("%s: %d total (INFO=%d, ERROR=%d, WARN=%d)",
			s.node, s.total, s.types["[INFO]"], s.types["[ERROR]"], s.types["[WARN]"],
		)
		total += s.total
	}

	log.Printf("%d total log entries processed", total)

	return nil
}

type Worker struct {
	queue chan Job
}

func (w *Worker) Start(ctx context.Context) error {
	// receive job
	for job := range w.queue {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			elements := strings.Split(job.work, " ")
			if len(elements) < 5 {
				// optional: return error or send to DLQ
				log.Printf("invalid log entry: %s", job.work)
				continue
			}
			// index 1: type, index 3: node name
			t := elements[1]
			n := elements[3]

			job.log.Update(n, t)
		}
	}
	return nil
}

type WorkerPool struct {
	workers []*Worker
	*errgroup.Group
}

type Job struct {
	work string
	log  *Log
}

func (wp *WorkerPool) SubmitJob(job Job) {
	// random worker for simplicity; round-robin or load-based in prod
	i := rand.Intn(len(wp.workers))
	wp.workers[i].queue <- job // buffered channels reduce backpressure
}

func (wp *WorkerPool) Close() {
	for _, w := range wp.workers {
		close(w.queue)
	}
}

func NewWorkerPool(ctx context.Context, maxConcurrency, bufferSize int) *WorkerPool {
	workers := make([]*Worker, 0, maxConcurrency)
	for range maxConcurrency {
		workers = append(workers, &Worker{queue: make(chan Job, bufferSize)}) // higher throughput, same goroutine count
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	wp := &WorkerPool{workers, g}

	for _, worker := range wp.workers {
		g.Go(func() error {
			return worker.Start(ctx)
		})
	}

	return wp
}

func main() {
	var inputLog string
	flag.StringVar(&inputLog, "in", "log.txt", "input file path (default log.txt)")

	var maxConcurrency int
	flag.IntVar(&maxConcurrency, "concurrency", 5, "number of workers in the worker pool (default: 5)")

	var bufferSize int
	flag.IntVar(&bufferSize, "buffer", 100, "channel buffer size per worker (default: 100)")

	var sortBy string
	flag.StringVar(&sortBy, "sortBy", "total", "name | total")

	var outputFormat string
	flag.StringVar(&outputFormat, "out", "print", "print | file")

	flag.Parse()

	// read input file stream (logs.txt)
	file, err := os.Open(inputLog)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	nodeLog := NewLog()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	wp := NewWorkerPool(ctx, maxConcurrency, bufferSize)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		wp.SubmitJob(Job{
			work: scanner.Text(),
			log:  nodeLog,
		})
	}

	// exit early from critical errors (i.e. malformed file)
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	// wait until all channels are drained
	wp.Close()
	if err := wp.Wait(); err != nil {
		log.Printf("first worker error: %v", err)
	}

	if err := nodeLog.WriteSummary(sortBy, outputFormat); err != nil {
		log.Fatal(err)
	}

	log.Println("terminated application gracefully")
}
