package loadgen

import (
	"bytes"
	"context"
	"expvar"
	"fmt"
	"math"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/nsf/termbox-go"
	"github.com/vearutop/dynhist-go"
	"golang.org/x/time/rate"
)

const (
	plotTailSize   = 48
	maxConcurrency = 1000
)

type runner struct {
	concurrencyLimit int64
	rateLimit        int64
	currentReqRate   int64
	errCnt           int64

	start time.Time
	exit  chan os.Signal

	roundTripHist    dynhist.Collector
	roundTripRolling dynhist.Collector
	roundTripPrecise dynhist.Collector

	jobProducer JobProducer

	mu      sync.Mutex
	rl      *rate.Limiter
	lastErr error
}

// Run runs load testing.
func Run(lf Flags, jobProducer JobProducer) error {
	if lf.LiveUI {
		if err := ui.Init(); err != nil {
			return fmt.Errorf("failed to initialize termui: %w", err)
		}
	}

	if lf.Output == nil {
		lf.Output = os.Stdout
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

	semaphore := make(chan struct{}, maxConcurrency)
	for i := 0; int64(i) < maxConcurrency-r.concurrencyLimit; i++ {
		semaphore <- struct{}{}
	}

	r.start = time.Now()
	slow := expvar.Int{}

	n := lf.Number
	if n <= 0 {
		n = math.MaxInt32
	}

	dur := lf.Duration
	if dur == 0 {
		dur = 1000 * time.Hour
	}

	if lf.RateLimit > 0 {
		r.rateLimit = int64(lf.RateLimit)
		r.rl = rate.NewLimiter(rate.Limit(lf.RateLimit), 1)
	}

	r.exit = make(chan os.Signal, 1)
	signal.Notify(r.exit, syscall.SIGTERM, os.Interrupt)

	done := int32(0)

	if lf.LiveUI {
		go r.startLiveUIPoller(semaphore)
	}

	go func() {
		for {
			<-r.exit

			if atomic.LoadInt32(&done) == 1 {
				os.Exit(1)
			}

			_, _ = fmt.Fprintln(lf.Output, "Stopping... Press CTRL+C again to force exit.")

			atomic.StoreInt32(&done, 1)
		}
	}()

	if lf.LiveUI {
		go r.runLiveUI()
	}

	for i := 0; i < n; i++ {
		i := i

		r.mu.Lock()
		rl := r.rl
		r.mu.Unlock()

		if rl != nil {
			err := rl.Wait(context.Background())
			if err != nil {
				r.mu.Lock()
				r.lastErr = err
				r.mu.Unlock()
			}
		}
		semaphore <- struct{}{} // Acquire semaphore slot.

		go func() {
			defer func() {
				<-semaphore // Release semaphore slot.
			}()

			elapsed, err := jobProducer.Job(i)
			if err != nil {
				r.mu.Lock()
				r.lastErr = err
				r.mu.Unlock()

				atomic.AddInt64(&r.errCnt, 1)

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
		semaphore <- struct{}{}
	}

	if lf.LiveUI {
		var buf []byte

		w, h := termbox.Size()

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := termbox.GetCell(x, y)

				buf = append(buf, []byte(string(c.Ch))...)
			}

			buf = bytes.Trim(buf, "\n \000")
			buf = append(buf, '\n')
		}

		ui.Close()

		buf = bytes.Trim(buf, "\n \000")
		_, _ = fmt.Fprintln(lf.Output, string(buf))
	}

	_, _ = fmt.Fprintln(lf.Output)
	_, _ = fmt.Fprintln(lf.Output, "Requests per second:", fmt.Sprintf("%.2f", float64(r.roundTripHist.Count)/time.Since(r.start).Seconds()))
	_, _ = fmt.Fprintln(lf.Output, "Successful requests:", r.roundTripHist.Count)

	if r.errCnt > 0 {
		_, _ = fmt.Fprintf(lf.Output, "Failed requests: %d, last error: %s\n", r.errCnt, r.lastErr.Error())
	}

	_, _ = fmt.Fprintln(lf.Output, "Time spent:", time.Since(r.start).Round(time.Millisecond))

	if r.roundTripHist.Count == 0 {
		return fmt.Errorf("all requests failed: %w", r.lastErr)
	}

	_, _ = fmt.Fprintln(lf.Output)
	_, _ = fmt.Fprintln(lf.Output, "Request latency percentiles:")
	_, _ = fmt.Fprintf(lf.Output, "99%%: %.2fms\n", r.roundTripPrecise.Percentile(99))
	_, _ = fmt.Fprintf(lf.Output, "95%%: %.2fms\n", r.roundTripPrecise.Percentile(95))
	_, _ = fmt.Fprintf(lf.Output, "90%%: %.2fms\n", r.roundTripPrecise.Percentile(90))
	_, _ = fmt.Fprintf(lf.Output, "50%%: %.2fms\n\n", r.roundTripPrecise.Percentile(50))

	_, _ = fmt.Fprintln(lf.Output, "Request latency distribution in ms:")
	_, _ = fmt.Fprintln(lf.Output, r.roundTripHist.String())

	_, _ = fmt.Fprintln(lf.Output, "Requests with latency more than "+lf.SlowResponse.String()+":", slow.Value())

	if s, ok := jobProducer.(fmt.Stringer); ok {
		_, _ = fmt.Fprintln(lf.Output, "\n"+s.String())
	}

	return nil
}

func (r *runner) startLiveUIPoller(limiter chan struct{}) {
	refreshRateLimiter := func() {
		lim := atomic.LoadInt64(&r.rateLimit)
		if lim == 0 {
			return
		}

		rl := rate.NewLimiter(rate.Limit(lim), 1)

		r.mu.Lock()
		r.rl = rl
		r.mu.Unlock()
	}

	rateLimit := func() int64 {
		lim := atomic.LoadInt64(&r.rateLimit)

		if lim == 0 {
			lim = atomic.LoadInt64(&r.currentReqRate)
		}

		return lim
	}

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

		case "<Up>": // Increase rate limit.
			lim := rateLimit()

			delta := int64(0.05 * float64(lim))
			if delta < 1 {
				delta = 1
			}

			atomic.StoreInt64(&r.rateLimit, lim+delta)
			refreshRateLimiter()

		case "<Down>": // Decrease rate limit.
			lim := rateLimit()

			delta := int64(0.05 * float64(lim))
			if delta < 1 {
				delta = 1
			}

			atomic.StoreInt64(&r.rateLimit, lim-delta)
			refreshRateLimiter()
		}
	}
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

	tickerDuration := 500 * time.Millisecond
	ticker := time.NewTicker(tickerDuration).C

	prev := time.Now()
	reqPrev := 0

	for {
		doReturn := false
		select {
		case <-ticker:
		case <-r.exit:
			doReturn = true
		}

		elaTick := time.Since(prev)
		reqNum := r.roundTripHist.Count

		drawables := make([]ui.Drawable, 0, 2)
		elaDur := time.Since(r.start)
		ela := elaDur.Seconds()
		reqRate := float64(reqNum) / ela
		reqRateTick := float64(reqNum-reqPrev) / elaTick.Seconds()
		reqPrev = reqNum
		prev = time.Now()

		atomic.StoreInt64(&r.currentReqRate, int64(reqRateTick))

		latencyPercentiles := widgets.NewParagraph()
		latencyPercentiles.Title = " Round trip latency, ms "
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

		if errCnt := atomic.LoadInt64(&r.errCnt); errCnt != 0 {
			counts["err"] = int(errCnt)
		}

		requestCounters := widgets.NewParagraph()
		requestCounters.Title = " Request Count "
		requestCounters.Text = ""

		requestCounters.SetRect(30, 0, 60, 7)

		lastErr := ""

		r.mu.Lock()
		if r.lastErr != nil {
			lastErr = "ERR: " + r.lastErr.Error()
			r.lastErr = nil
		}
		r.mu.Unlock()

		loadLimits := widgets.NewParagraph()
		loadLimits.Title = " Load Limits "
		loadLimits.Text = fmt.Sprintf("Concurrency: %d, <Right>/<Left>: ±5%%\nRate Limit: %d, <Up>/<Down>: ±5%%\n%s",
			atomic.LoadInt64(&r.concurrencyLimit), atomic.LoadInt64(&r.rateLimit), lastErr)

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

		rpsPlot.Title = fmt.Sprintf(" Press Q or Ctrl+C to quit | avg rps: %.2f, current rps: %.2f, elapsed: %s\n",
			reqRate,
			reqRateTick,
			elaDur.Round(tickerDuration).String(),
		)

		latencyPlot.Data[0] = append(latencyPlot.Data[0], r.roundTripRolling.Min)
		latencyPlot.Data[1] = append(latencyPlot.Data[1], r.roundTripRolling.Max)

		if len(latencyPlot.Data[0]) > plotTailSize {
			latencyPlot.Data[0] = latencyPlot.Data[0][len(latencyPlot.Data[0])-plotTailSize:]
			latencyPlot.Data[1] = latencyPlot.Data[1][len(latencyPlot.Data[1])-plotTailSize:]
		}

		latencyPlot.Title = " Min/Max Latency, ms "

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
