package loadgen

import (
	"context"
	"expvar"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"sort"
	"sync/atomic"
	"syscall"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/vearutop/dynhist-go"
	"golang.org/x/time/rate"
)

const (
	plotTailSize   = 48
	maxConcurrency = 1000
)

type runner struct {
	start time.Time
	exit  chan os.Signal

	roundTripHist    dynhist.Collector
	roundTripRolling dynhist.Collector
	roundTripPrecise dynhist.Collector

	jobProducer JobProducer

	concurrencyLimit int64
}

// Run runs load testing.
func Run(lf Flags, jobProducer JobProducer) {
	if lf.LiveUI {
		if err := ui.Init(); err != nil {
			log.Fatalf("failed to initialize termui: %v", err)
		}
	}

	r := runner{
		jobProducer:      jobProducer,
		roundTripHist:    dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth},
		roundTripRolling: dynhist.Collector{BucketsLimit: 5, WeightFunc: dynhist.LatencyWidth},
		roundTripPrecise: dynhist.Collector{BucketsLimit: 100, WeightFunc: dynhist.LatencyWidth},
	}

	r.concurrencyLimit = int64(lf.Concurrency) // Number of simultaneous jobs.
	if r.concurrencyLimit <= 0 {
		r.concurrencyLimit = 50
	}

	limiter := make(chan struct{}, maxConcurrency)
	for i := 0; int64(i) < maxConcurrency-r.concurrencyLimit; i++ {
		limiter <- struct{}{}
	}

	r.start = time.Now()
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
		rl = rate.NewLimiter(rate.Limit(lf.RateLimit), int(r.concurrencyLimit))
	}

	r.exit = make(chan os.Signal, 1)
	signal.Notify(r.exit, syscall.SIGTERM, os.Interrupt)

	done := int32(0)

	if lf.LiveUI {
		go func() {
			uiEvents := ui.PollEvents()
			for e := range uiEvents {
				switch e.ID {
				case "q", "<C-c>":
					r.exit <- os.Interrupt
				case "<Right>": // Increase concurrency.
					lim := atomic.LoadInt64(&r.concurrencyLimit)
					delta := int64(0.05 * float64(lim))
					if lim+delta <= maxConcurrency {
						atomic.AddInt64(&r.concurrencyLimit, delta)
						for i := int64(0); i < delta; i++ {
							<-limiter
						}
					}
				case "<Left>": // Decrease concurrency.
					lim := atomic.LoadInt64(&r.concurrencyLimit)
					delta := int64(0.05 * float64(lim))
					if lim-delta > 0 {
						atomic.AddInt64(&r.concurrencyLimit, -delta)
						for i := int64(0); i < delta; i++ {
							limiter <- struct{}{}
						}
					}
				}
			}
		}()
	}

	go func() {
		<-r.exit
		atomic.StoreInt32(&done, 1)
	}()

	if lf.LiveUI {
		go r.runLiveUI()
	}

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

			r.roundTripHist.Add(ms)
			r.roundTripPrecise.Add(ms)
			r.roundTripRolling.Add(ms)
		}()

		if time.Since(r.start) > dur || atomic.LoadInt32(&done) == 1 {
			break
		}
	}

	// Wait for goroutines to finish by filling full channel.
	for i := 0; int64(i) < atomic.LoadInt64(&r.concurrencyLimit); i++ {
		limiter <- struct{}{}
	}

	if lf.LiveUI {
		ui.Close()
	}

	fmt.Println()
	fmt.Println("Requests per second:", fmt.Sprintf("%.2f", float64(r.roundTripHist.Count)/time.Since(r.start).Seconds()))
	fmt.Println("Total requests:", r.roundTripHist.Count)
	fmt.Println("Time spent:", time.Since(r.start))

	fmt.Println()
	fmt.Println("Request latency percentiles:")
	fmt.Printf("99%%: %.2fms\n", r.roundTripPrecise.Percentile(99))
	fmt.Printf("95%%: %.2fms\n", r.roundTripPrecise.Percentile(95))
	fmt.Printf("90%%: %.2fms\n", r.roundTripPrecise.Percentile(90))
	fmt.Printf("50%%: %.2fms\n\n", r.roundTripPrecise.Percentile(50))

	fmt.Println("Request latency distribution in ms:")
	fmt.Println(r.roundTripHist.String())

	fmt.Println("Requests with latency more than "+lf.SlowResponse.String()+":", slow.Value())
}

func (r *runner) runLiveUI() {
	rates := make(map[string][]float64, 10)

	rpsPlot := widgets.NewPlot()
	rpsPlot.SetRect(0, 7, 100, 15)
	rpsPlot.ShowAxes = false
	rpsPlot.HorizontalScale = 2

	latencyPlot := widgets.NewPlot()
	latencyPlot.SetRect(0, 15, 100, 35)
	latencyPlot.Data = [][]float64{0: {}, 1: {}}
	latencyPlot.HorizontalScale = 2

	ticker := time.NewTicker(500 * time.Millisecond).C

	for {
		doReturn := false
		select {
		case <-ticker:
		case <-r.exit:
			doReturn = true
		}

		drawables := make([]ui.Drawable, 0, 2)
		elaDur := time.Since(r.start)
		ela := elaDur.Seconds()
		reqRate := float64(r.roundTripHist.Count) / ela

		latencyPercentiles := widgets.NewParagraph()
		latencyPercentiles.Title = "Round trip latency, ms"
		latencyPercentiles.Text = ""

		latencyPercentiles.Text += fmt.Sprintf("100%%: %.2fms\n", r.roundTripPrecise.Percentile(100))
		latencyPercentiles.Text += fmt.Sprintf("99%%: %.2fms\n", r.roundTripPrecise.Percentile(99))
		latencyPercentiles.Text += fmt.Sprintf("95%%: %.2fms\n", r.roundTripPrecise.Percentile(95))
		latencyPercentiles.Text += fmt.Sprintf("90%%: %.2fms\n", r.roundTripPrecise.Percentile(90))
		latencyPercentiles.Text += fmt.Sprintf("50%%: %.2fms\n", r.roundTripPrecise.Percentile(50))
		latencyPercentiles.SetRect(0, 0, 30, 7)

		drawables = append(drawables, latencyPercentiles)

		counts := r.jobProducer.RequestCounts()
		counts["tot"] = r.roundTripHist.Count

		requestCounters := widgets.NewParagraph()
		requestCounters.Title = "Request Count"
		requestCounters.Text = ""

		requestCounters.SetRect(30, 0, 60, 7)

		loadLimits := widgets.NewParagraph()
		loadLimits.Title = "Load Limits"
		loadLimits.Text = fmt.Sprintf("Concurrency: %d, <Right>/<Left>: ±5%%", atomic.LoadInt64(&r.concurrencyLimit))

		loadLimits.SetRect(60, 0, 100, 7)

		drawables = append(drawables, requestCounters, loadLimits)

		rpsPlot.DataLabels = make([]string, 0, len(counts))
		rpsPlot.Data = make([][]float64, 0, len(counts))

		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, name := range keys {
			cnt := counts[name]
			requestCounters.Text += fmt.Sprintf("%s: %d\n", name, cnt)

			rates[name] = append(rates[name], float64(cnt)/ela)
			if len(rates[name]) < 2 {
				continue
			}

			if len(rates[name]) > plotTailSize {
				rates[name] = rates[name][len(rates[name])-plotTailSize:]
			}

			rpsPlot.DataLabels = append(rpsPlot.DataLabels, name)
			rpsPlot.Data = append(rpsPlot.Data, rates[name])
		}

		rpsPlot.Title = "Requests per second:" + fmt.Sprintf("%.2f (total requests: %d, time passed: %s)\n",
			reqRate,
			r.roundTripHist.Count,
			elaDur.String(),
		)

		latencyPlot.Data[0] = append(latencyPlot.Data[0], r.roundTripRolling.Min)
		latencyPlot.Data[1] = append(latencyPlot.Data[1], r.roundTripRolling.Max)

		if len(latencyPlot.Data[0]) > plotTailSize {
			latencyPlot.Data[0] = latencyPlot.Data[0][len(latencyPlot.Data[0])-plotTailSize:]
			latencyPlot.Data[1] = latencyPlot.Data[1][len(latencyPlot.Data[1])-plotTailSize:]
		}

		latencyPlot.Title = "Min/Max Latency, ms"

		if len(latencyPlot.Data[0]) > 1 {
			drawables = append(drawables, latencyPlot)
		}

		drawables = append(drawables, rpsPlot)

		ui.Render(drawables...)
		r.roundTripRolling.Lock()
		r.roundTripRolling.Buckets = nil
		r.roundTripRolling.Count = 0
		r.roundTripRolling.Min = 0
		r.roundTripRolling.Max = 0
		r.roundTripRolling.Unlock()

		if doReturn {
			return
		}
	}
}
