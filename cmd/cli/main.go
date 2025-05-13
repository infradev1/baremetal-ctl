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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	concurrencyLimit = 10
	deadline         = 5 * time.Second
	retries          = 3
)

func main() {
	inputFile := os.Args[1]  // validate input
	outputFile := os.Args[2] // validate input

	file, err := os.Open(inputFile) // could OOM if a large file is read directly
	if err != nil {
		log.Fatalf("failed to open input file %s: %v", inputFile, err)
	}
	defer file.Close()

	out, err := os.Create(outputFile) // careful about concurrent writes
	if err != nil {
		log.Fatalf("failed to create output file %s: %v", outputFile, err)
	}
	defer out.Close()

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrencyLimit)

	urls := make(chan string, concurrencyLimit)
	responses := make(chan string)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		urls <- scanner.Text()

		g.Go(func() error {
			start := time.Now()

			// optional: wrap in exponential backoff with retries
			c, cancel := context.WithTimeout(ctx, deadline)
			defer cancel()

			req, err := http.NewRequestWithContext(c, http.MethodGet, <-urls, nil)
			if err != nil {
				return fmt.Errorf("error creating request: %w", err)
			}

			for i := range retries {
				if i == retries {
					return fmt.Errorf("request failed after %d retries", retries)
				}

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return fmt.Errorf("error getting response: %w", err)
				}
				if resp.StatusCode >= 500 || resp.StatusCode < 600 {
					continue
				}

				responses <- fmt.Sprintf("Status Code: %d, Request Duration: %s", resp.StatusCode, time.Since(start).String())
				break
			}

			return nil
		})
	}

	// Writer goroutine
	go func() {
		for r := range responses {
			if _, err := fmt.Fprintf(out, "%s", r); err != nil {
				slog.Error("failed to write error status code string to output file", slog.String("error", err.Error()))
			}
		}
	}()

	if err = g.Wait(); err != nil {
		slog.Error("GET failed", slog.String("error", err.Error()))
	}
}
