package loadgen

import (
	"context"
	"expvar"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vearutop/dynhist-go"
	"golang.org/x/time/rate"
)

func Run(lf Flags, jobProducer JobProducer) {
	roundTripHist := dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}

	concurrencyLimit := lf.Concurrency // Number of simultaneous jobs.
	if concurrencyLimit <= 0 {
		concurrencyLimit = 50
	}

	limiter := make(chan struct{}, concurrencyLimit)

	start := time.Now()
	slow := expvar.Int{}

	n := lf.Number
	if n <= 0 {
		n = math.MaxInt64
	}
	dur := lf.Duration
	if dur == 0 {
		dur = 1000 * time.Hour
	}

	var rl *rate.Limiter
	if lf.RateLimit > 0 {
		rl = rate.NewLimiter(rate.Limit(lf.RateLimit), concurrencyLimit)
	}

	exit := make(chan os.Signal)
	signal.Notify(exit, syscall.SIGTERM, os.Interrupt)
	done := int32(0)
	go func() {
		<-exit
		atomic.StoreInt32(&done, 1)
	}()

	println("Starting")
	for i := 0; i < n; i++ {
		if rl != nil {
			err := rl.Wait(context.Background())
			if err != nil {
				log.Println(err.Error())
			}
		}
		limiter <- struct{}{} // Reserve limiter slot.
		go func() {
			defer func() {
				<-limiter // Free limiter slot.
			}()

			elapsed, err := jobProducer.Job(i) // err
			if err != nil {
				log.Println(err.Error())
				return
			}
			ms := elapsed.Seconds() * 1000
			if elapsed >= lf.SlowResponse {
				slow.Add(1)
			}
			roundTripHist.Add(ms)
		}()

		if time.Since(start) > dur || atomic.LoadInt32(&done) == 1 {
			break
		}
	}

	// Wait for goroutines to finish by filling full channel.
	for i := 0; i < cap(limiter); i++ {
		limiter <- struct{}{}
	}

	println("Requests per second:", fmt.Sprintf("%.2f", float64(roundTripHist.Count)/time.Since(start).Seconds()))
	println("Total requests:", roundTripHist.Count)
	println("Request latency distribution in ms:")
	println(roundTripHist.String())
	println("Requests with latency more than "+lf.SlowResponse.String()+":", slow.Value())
}
