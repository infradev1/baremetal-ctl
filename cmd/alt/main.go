package main

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// type aliases for readability
type NodeName = string
type StatusCounts = map[string]int // i.e. "ERROR": 3

type Log struct {
	nodeInfo map[NodeName]StatusCounts
	sync.Mutex
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

type LogSummary struct {
	node  NodeName
	total int
	types map[string]int
}

// gpu-node-17: 5 total (INFO=3, ERROR=1, WARN=1)
// gpu-node-32: 2 total (INFO=1, ERROR=1, WARN=0)
func (l *Log) PrintSummary() {
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
		return cmp.Compare(b.total, a.total) // descending order
	})

	for _, s := range summary {
		log.Printf("%s: %d total (INFO=%d, ERROR=%d, WARN=%d)",
			s.node, s.total, s.types["[INFO]"], s.types["[ERROR]"], s.types["[WARN]"],
		)
	}
}

func main() {
	if len(os.Args) != 3 {
		log.Fatal("required format: go run main.go log.txt n, where n is max concurrency")
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

	nodeLog := NewLog()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		g.Go(func() error {
			// call function that accepts the context, with deadline if HTTP requests for example
			elements := strings.Split(line, " ")
			if len(elements) < 5 {
				return errors.New("invalid log entry")
			}
			// index 1: type, index 3: node name
			t := elements[1]
			n := elements[3]

			nodeLog.Update(n, t)

			return nil
		})
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	if err := g.Wait(); err != nil {
		log.Printf("error parsing entry: %v", err)
	}

	nodeLog.PrintSummary()
}
