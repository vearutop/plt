package loadgen

import (
	"fmt"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"log"
	"os"
	"time"
)

type dashboard struct {
}

func (d *dashboard) AddLatency() {

}

func (d *dashboard) Metric() {

}

func (d *dashboard) startUI(exit chan os.Signal) {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

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

	rates := make(map[string][]float64, 10)

	lc2 := widgets.NewPlot()
	lc2.SetRect(0, 5, 100, 15)
	lc2.ShowAxes = true

	//lc3 := widgets.NewPlot()
	//lc3.SetRect(0, 15, 100, 25)
	//lc3.ShowAxes = true

	ticker := time.NewTicker(500 * time.Millisecond).C
	for {
		select {
		case <-ticker:
		case <-exit:
			ui.Close()
			return
		}

		ela := time.Since(start).Seconds()
		reqRate := float64(roundTripHist.Count) / ela

		p := widgets.NewParagraph()
		p.Title = "Round trip latency, ms (press q or ctrl+c to quit)"
		p.Text = ""

		p.Text += fmt.Sprintf("100%%: %fms\n", roundTripHist.Percentile(1))
		p.Text += fmt.Sprintf("99%%: %fms\n", roundTripHist.Percentile(0.99))
		p.Text += fmt.Sprintf("95%%: %fms\n", roundTripHist.Percentile(0.95))
		p.Text += fmt.Sprintf("90%%: %fms\n", roundTripHist.Percentile(0.90))
		p.Text += fmt.Sprintf("50%%: %fms\n", roundTripHist.Percentile(0.50))
		p.SetRect(0, 0, 100, 5)
		//p.TextStyle.Fg = ui.ColorWhite
		//p.BorderStyle.Fg = ui.ColorCyan

		counts := jobProducer.Counts()
		counts["tot"] = roundTripHist.Count
		lc2.DataLabels = make([]string, 0, len(counts))
		lc2.Data = make([][]float64, 0, len(counts))
		for name, cnt := range counts {
			rates[name] = append(rates[name], float64(cnt)/ela)
			if len(rates[name]) < 2 {
				continue
			}
			if len(rates[name]) > 90 {
				rates[name] = rates[name][len(rates[name])-90:]
			}
			lc2.DataLabels = append(lc2.DataLabels, name)
			lc2.Data = append(lc2.Data, rates[name])
		}

		lc2.Title = "Requests per second:" + fmt.Sprintf("%.2f (%d)\n",
			reqRate,
			roundTripHist.Count,
		)

		ui.Render(p, lc2)
	}
}
