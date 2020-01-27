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
	"sort"
	"sync/atomic"
	"syscall"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/vearutop/dynhist-go"
	"golang.org/x/time/rate"
)

const plotTailSize = 48

func Run(lf Flags, jobProducer JobProducer) {
	if lf.LiveUI {
		if err := ui.Init(); err != nil {
			log.Fatalf("failed to initialize termui: %v", err)
		}
	}

	roundTripHist := dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	roundTripRolling := dynhist.Collector{BucketsLimit: 5, WeightFunc: dynhist.LatencyWidth}
	roundTripPrecise := dynhist.Collector{BucketsLimit: 100, WeightFunc: dynhist.LatencyWidth}

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

	if lf.LiveUI {
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
	}

	go func() {
		<-exit
		atomic.StoreInt32(&done, 1)
	}()

	if lf.LiveUI {
		go func() {

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
				case <-exit:
					doReturn = true
				}
				drawables := make([]ui.Drawable, 0, 2)
				elaDur := time.Since(start)
				ela := elaDur.Seconds()
				reqRate := float64(roundTripHist.Count) / ela

				latencyPercentiles := widgets.NewParagraph()
				latencyPercentiles.Title = "Round trip latency, ms"
				latencyPercentiles.Text = ""

				latencyPercentiles.Text += fmt.Sprintf("100%%: %fms\n", roundTripPrecise.Percentile(100))
				latencyPercentiles.Text += fmt.Sprintf("99%%: %fms\n", roundTripPrecise.Percentile(99))
				latencyPercentiles.Text += fmt.Sprintf("95%%: %fms\n", roundTripPrecise.Percentile(95))
				latencyPercentiles.Text += fmt.Sprintf("90%%: %fms\n", roundTripPrecise.Percentile(90))
				latencyPercentiles.Text += fmt.Sprintf("50%%: %fms\n", roundTripPrecise.Percentile(50))
				latencyPercentiles.SetRect(0, 0, 30, 7)

				drawables = append(drawables, latencyPercentiles)

				counts := jobProducer.RequestCounts()
				counts["tot"] = roundTripHist.Count

				requestCounters := widgets.NewParagraph()
				requestCounters.Title = "Request Count"
				requestCounters.Text = ""

				requestCounters.SetRect(30, 0, 60, 7)

				drawables = append(drawables, requestCounters)

				rpsPlot.DataLabels = make([]string, 0, len(counts))
				rpsPlot.Data = make([][]float64, 0, len(counts))

				keys := make([]string, 0, len(counts))
				for k, _ := range counts {
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
					roundTripHist.Count,
					elaDur.String(),
				)

				latencyPlot.Data[0] = append(latencyPlot.Data[0], roundTripRolling.Min)
				latencyPlot.Data[1] = append(latencyPlot.Data[1], roundTripRolling.Max)
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
				roundTripRolling.Lock()
				roundTripRolling.Buckets = nil
				roundTripRolling.Count = 0
				roundTripRolling.Min = 0
				roundTripRolling.Max = 0
				roundTripRolling.Unlock()

				if doReturn {
					return
				}
			}
		}()
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
			roundTripHist.Add(ms)
			roundTripPrecise.Add(ms)
			roundTripRolling.Add(ms)
		}()

		if time.Since(start) > dur || atomic.LoadInt32(&done) == 1 {
			break
		}
	}

	// Wait for goroutines to finish by filling full channel.
	for i := 0; i < cap(limiter); i++ {
		limiter <- struct{}{}
	}

	if lf.LiveUI {
		ui.Close()
	}

	fmt.Println("Requests per second:", fmt.Sprintf("%.2f", float64(roundTripHist.Count)/time.Since(start).Seconds()))
	fmt.Println("Total requests:", roundTripHist.Count)
	fmt.Println("Request latency distribution in ms:")
	fmt.Println(roundTripHist.String())
	fmt.Println("Request latency percentiles:")
	fmt.Printf("99%%: %fms\n", roundTripPrecise.Percentile(99))
	fmt.Printf("95%%: %fms\n", roundTripPrecise.Percentile(95))
	fmt.Printf("90%%: %fms\n", roundTripPrecise.Percentile(90))
	fmt.Printf("50%%: %fms\n\n", roundTripPrecise.Percentile(50))
	fmt.Println("Requests with latency more than "+lf.SlowResponse.String()+":", slow.Value())

}
