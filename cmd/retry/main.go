package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	maxConcurrency = 10
	retries        = 3
	timeout        = 5 * time.Second
)

type result struct {
	url        string
	statusCode int
	duration   time.Duration
	err        error
}

func main() {
	if len(os.Args) != 3 {
		log.Fatal("required flags missing: i.e. go run main.go input.txt output.txt")
	}

	inFile := os.Args[1]
	outFile := os.Args[2]

	file, err := os.Open(inFile)
	if err != nil {
		log.Fatalf("failed to open input file: %s", inFile)
	}
	defer file.Close()

	out, err := os.Create(outFile)
	if err != nil {
		log.Fatalf("failed to create output file: %s", outFile)
	}
	defer out.Close()

	// goroutines will send to this channel
	// a standalone goroutine will receive from this channel until the sender closes it
	results := make(chan result, maxConcurrency)

	// wait until results channel is drained
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		w := bufio.NewWriter(out)
		defer w.Flush()
		lineCount := 0

		for r := range results {
			if r.err != nil {
				if _, err := fmt.Fprintf(w, "%s, %v\n", r.url, r.err); err != nil {
					log.Fatal(err)
				}
			} else {
				if _, err := fmt.Fprintf(w, "%s, %d, %s\n", r.url, r.statusCode, r.duration); err != nil {
					log.Fatal(err)
				}
			}
			lineCount++
			if lineCount%100 == 0 {
				w.Flush() // flush in intervals to lower memory pressure
			}
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	// main writer logic
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrency)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		url := scanner.Text()

		g.Go(func() error {
			start := time.Now()
			c, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			// process url
			code, err := process(c, url)
			// send to results channel
			results <- result{
				url:        url,
				statusCode: code,
				duration:   time.Since(start),
				err:        err,
			}

			return nil
		})
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	if err := g.Wait(); err != nil {
		log.Println(err)
	}

	close(results)
	wg.Wait()
}

// process returns status code or error
func process(ctx context.Context, url string) (int, error) {
	var lastError error

	for range retries {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastError = err
			continue
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			lastError = err
			continue
		}
		defer res.Body.Close()

		if res.StatusCode >= 500 && res.StatusCode <= 599 {
			continue
		}

		return res.StatusCode, nil
	}

	return 0, lastError
}
