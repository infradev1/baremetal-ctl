package main

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

type WorkerPool struct {
	workers []Worker
}

type Job struct {
	work string
	log  *Log
	wg   *sync.WaitGroup
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

func NewWorkerPool(maxConcurrency int) *WorkerPool {
	workers := make([]Worker, 0, maxConcurrency)
	for range maxConcurrency {
		workers = append(workers, Worker{queue: make(chan Job, 100)}) // higher throughput, same goroutine count
	}
	wp := &WorkerPool{workers: workers}

	for _, w := range wp.workers {
		go func() {
			// receive job
			for job := range w.queue {
				// call function that accepts the context, with deadline if HTTP requests for example
				elements := strings.Split(job.work, " ")
				if len(elements) < 5 {
					log.Printf("invalid log entry: %s", job.work)
					continue
				}
				// index 1: type, index 3: node name
				t := elements[1]
				n := elements[3]

				job.log.Update(n, t)
				job.wg.Done()
			}
		}()
	}

	return wp
}

func main() {
	if len(os.Args) != 5 {
		log.Fatal("required format: go run main.go log.txt n sortBy, where n is max concurrency, sortBy: name | total, and output: print | file")
	}
	// read input file stream (logs.txt)
	file, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	maxConcurrency, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}

	sortBy := os.Args[3]
	outputFormat := os.Args[4]

	nodeLog := NewLog()
	wp := NewWorkerPool(maxConcurrency)
	wg := &sync.WaitGroup{}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		log.Println("closing worker channels")
		wp.Close()
		log.Println("terminated application gracefully")
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		wg.Add(1)
		wp.SubmitJob(Job{
			work: scanner.Text(),
			log:  nodeLog,
			wg:   wg,
		})
	}
	wg.Wait()

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	if err := nodeLog.WriteSummary(sortBy, outputFormat); err != nil {
		log.Fatal(err)
	}
}
