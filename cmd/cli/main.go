/*
Build a Go CLI that reads a file containing a list of URLs (1 per line),
performs HTTP GET requests to each URL concurrently, and writes the status code
and duration of each request to an output file. Limit concurrency to 10 requests at a time.
Timeout each request after 5 seconds.

Follow-up curveballs:
What happens if the input file has 100,000 URLs? How would you optimize for memory?
How would you add graceful shutdown via context cancellation (e.g., Ctrl+C)?
How would you retry 5xx status codes up to 3 times?
*/
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
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	concurrencyLimit = 10
	requestTimeout   = 5 * time.Second
	maxRetries       = 3
)

type result struct {
	URL      string
	Status   int
	Duration time.Duration
	Err      error
}

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("Usage: %s input.txt output.txt", os.Args[0])
	}

	inputPath := os.Args[1]
	outputPath := os.Args[2]

	inFile, err := os.Open(inputPath)
	if err != nil {
		log.Fatalf("Failed to open input file: %v", err)
	}
	defer inFile.Close()

	outFile, err := os.Create(outputPath)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outFile.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	results := make(chan result, concurrencyLimit)

	var writeWG sync.WaitGroup
	writeWG.Add(1)

	go func() {
		defer writeWG.Done()
		writer := bufio.NewWriter(outFile)
		defer writer.Flush()

		for r := range results {
			if r.Err != nil {
				fmt.Fprintf(writer, "%s ERROR: %v\n", r.URL, r.Err)
			} else {
				fmt.Fprintf(writer, "%s %d %v\n", r.URL, r.Status, r.Duration)
			}
		}
	}()

	scanner := bufio.NewScanner(inFile)
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrencyLimit)

	for scanner.Scan() {
		url := scanner.Text() // capture by value
		g.Go(func() error {
			start := time.Now()
			status, err := fetchWithRetries(ctx, url)
			duration := time.Since(start)

			results <- result{
				URL:      url,
				Status:   status,
				Duration: duration,
				Err:      err,
			}
			return nil // return error only for fatal issues
		})
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input file: %v", err)
	}

	if err := g.Wait(); err != nil {
		log.Printf("One or more requests failed: %v", err)
	}

	close(results)
	writeWG.Wait()
}

func fetchWithRetries(ctx context.Context, url string) (int, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			return 0, fmt.Errorf("request creation failed: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
			lastErr = fmt.Errorf("server error %d", resp.StatusCode)
			continue
		}

		return resp.StatusCode, nil
	}

	return 0, fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
}
