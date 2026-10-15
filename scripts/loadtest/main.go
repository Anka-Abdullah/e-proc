// Command loadtest fires concurrent authenticated requests at the gateway and
// reports latency percentiles and throughput.
//
//	go run ./scripts/loadtest -n 500 -c 25 -path /api/v1/vendors?per_page=5
//
// It mints its own token, so no manual setup is needed. Use it to find the point
// where the edge rate limiter engages (RATE_LIMIT_RPS) as well as raw latency.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/config"
)

func main() {
	base := flag.String("base", "http://localhost:8000", "gateway base URL")
	path := flag.String("path", "/api/v1/vendors?per_page=5", "request path")
	requests := flag.Int("n", 500, "total requests")
	concurrency := flag.Int("c", 25, "concurrent workers")
	role := flag.String("role", string(auth.RoleAdmin), "role for the token")
	flag.Parse()

	cfg, err := config.Load("loadtest")
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	token, err := auth.Issue(cfg.JWTSecret, time.Hour, auth.Claims{
		UserID:   "u-loadtest",
		Email:    "loadtest@example.com",
		Role:     auth.Role(*role),
		TenantID: "demo",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "token: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        *concurrency * 2,
			MaxIdleConnsPerHost: *concurrency * 2,
		},
	}

	var (
		statuses  sync.Map // status code -> *int64
		latencies = make([]time.Duration, 0, *requests)
		latMu     sync.Mutex
		failures  int64
	)

	start := time.Now()
	jobs := make(chan int)
	var wg sync.WaitGroup

	for worker := 0; worker < *concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				reqCtx, cancelReq := context.WithTimeout(context.Background(), 15*time.Second)
				req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, *base+*path, nil)
				if err != nil {
					cancelReq()
					atomic.AddInt64(&failures, 1)
					continue
				}
				req.Header.Set("Authorization", "Bearer "+token)

				began := time.Now()
				response, err := client.Do(req)
				elapsed := time.Since(began)
				cancelReq()
				if err != nil {
					atomic.AddInt64(&failures, 1)
					continue
				}
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()

				counter, _ := statuses.LoadOrStore(response.StatusCode, new(int64))
				atomic.AddInt64(counter.(*int64), 1)

				latMu.Lock()
				latencies = append(latencies, elapsed)
				latMu.Unlock()
			}
		}()
	}

	for i := 0; i < *requests; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	total := time.Since(start)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	fmt.Printf("target      : %s%s\n", *base, *path)
	fmt.Printf("requests    : %d (concurrency %d)\n", *requests, *concurrency)
	fmt.Printf("duration    : %s\n", total.Round(time.Millisecond))
	fmt.Printf("throughput  : %.1f req/s\n", float64(*requests)/total.Seconds())
	fmt.Printf("failures    : %d (transport errors)\n", failures)

	if len(latencies) > 0 {
		fmt.Printf("latency p50 : %s\n", percentile(latencies, 0.50))
		fmt.Printf("latency p90 : %s\n", percentile(latencies, 0.90))
		fmt.Printf("latency p95 : %s\n", percentile(latencies, 0.95))
		fmt.Printf("latency p99 : %s\n", percentile(latencies, 0.99))
		fmt.Printf("latency max : %s\n", latencies[len(latencies)-1].Round(time.Microsecond))
	}

	fmt.Println("status codes:")
	statuses.Range(func(key, value any) bool {
		fmt.Printf("  %d: %d\n", key.(int), atomic.LoadInt64(value.(*int64)))
		return true
	})
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)-1) * p)
	return sorted[index].Round(time.Microsecond)
}
