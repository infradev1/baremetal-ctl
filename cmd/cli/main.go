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
	"sync"
	"time"
)

const (
	concurrencyLimit = 10
	deadline         = 5 * time.Second
)

func main() {
	inputFile := os.Args[1]
	outputFile := os.Args[2]
	urls := make([]string, 0)

	file, err := os.Open(inputFile) // could OOM if file is too large
	if err != nil {
		log.Fatalf("failed to open input file %s: %v", inputFile, err)
	}
	defer file.Close()

	out, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("failed to create output file %s: %v", outputFile, err)
	}
	defer out.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		urls = append(urls, scanner.Text())
	}

	sem := make(chan struct{}, concurrencyLimit)
	g := sync.WaitGroup{}
	c := context.Background()

	for _, url := range urls {
		sem <- struct{}{} // blocks when channel is full
		g.Add(1)

		go func(url string) {
			start := time.Now()

			ctx, cancel := context.WithTimeout(c, deadline)
			defer cancel()

			resp, err := http.Get(url)
			if err != nil {
				_, err := out.WriteString(fmt.Sprintf("Status Code: %d, Request Duration: %s", resp.StatusCode, time.Since(start).String()))
			}
		}(url)
	}
}
