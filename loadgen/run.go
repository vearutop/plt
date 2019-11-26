package loadgen

import (
	"context"
	"expvar"
	"fmt"
	"github.com/gizak/termui/v3/widgets"
	"log"
	"math"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/vearutop/dynhist-go"
	"golang.org/x/time/rate"
)

func Run(lf Flags, jobProducer JobProducer) {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}

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
		uiEvents := ui.PollEvents()
		for {
			select {
			case e := <-uiEvents:
				switch e.ID {
				case "q", "<C-c>":
					exit <- os.Interrupt
				}
			}
		}
	}()

	go func() {
		<-exit
		atomic.StoreInt32(&done, 1)
	}()

	go func() {

		latency := make([]float64, 1, 100)

		ticker := time.NewTicker(500 * time.Millisecond).C
		for {
			select {
			case <-ticker:
			case <-exit:
				return
			}

			rate := float64(roundTripHist.Count) / time.Since(start).Seconds()

			p := widgets.NewParagraph()
			p.Title = "Round trip latency (press q or ctrl+c to quit)"
			p.Text = ""

			p.Text += fmt.Sprintf("100%%: %fms\n", roundTripHist.Percentile(1))
			p.Text += fmt.Sprintf("99%%: %fms\n", roundTripHist.Percentile(0.99))
			p.Text += fmt.Sprintf("95%%: %fms\n", roundTripHist.Percentile(0.95))
			p.Text += fmt.Sprintf("90%%: %fms\n", roundTripHist.Percentile(0.90))
			p.Text += fmt.Sprintf("50%%: %fms\n", roundTripHist.Percentile(0.50))
			p.SetRect(0, 0, 100, 15)
			//p.TextStyle.Fg = ui.ColorWhite
			//p.BorderStyle.Fg = ui.ColorCyan

			latency = append(latency, rate)

			lc2 := widgets.NewPlot()
			lc2.Title = "Requests per second:" + fmt.Sprintf("%.2f (%d)\n",
				rate,
				roundTripHist.Count,
			)
			lc2.Data = make([][]float64, 1)
			lc2.Data[0] = latency
			lc2.SetRect(0, 15, 100, 25)
			lc2.AxesColor = ui.ColorWhite
			lc2.LineColors[0] = ui.ColorYellow

			ui.Render(p, lc2)
		}
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

	ui.Close()

	println("Requests per second:", fmt.Sprintf("%.2f", float64(roundTripHist.Count)/time.Since(start).Seconds()))
	println("Total requests:", roundTripHist.Count)
	println("Request latency distribution in ms:")
	println(roundTripHist.String())
	println("Requests with latency more than "+lf.SlowResponse.String()+":", slow.Value())
}
