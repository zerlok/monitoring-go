package main

import (
	"context"
	"github.com/zerlok/monitoring-go/sdk"
	"log"
	"os"
	"os/signal"
	"strings"
)

var (
	monitoring       = sdk.NewBuilder().Build()
	calculationGroup = monitoring.GroupName("calculation")
)

func fibonacci(ctx context.Context, n uint, dest chan<- uint) {
	scraper := calculationGroup.Scrape(ctx)
	defer scraper.End()

	defer close(dest)

	x, y := uint(0), uint(1)

	for i := uint(1); ctx.Err() == nil && i < n+1; i++ {
		dest <- x
		x, y = y, x+y

		scraper.AddEvent("next fibonacci calculated")
	}
}

func printer(ctx context.Context, src <-chan uint) {
	scraper := monitoring.Scrape(ctx)
	defer scraper.End()

	for {
		select {
		case <-ctx.Done():
			return
		case x, ok := <-src:
			if !ok {
				return
			}

			log.Printf("%d\n", x)
		}
	}
}

func main() {
	_ = monitoring.Setup()
	defer monitoring.Shutdown()

	runApp()
}

func runApp() {
	ctx := context.Background()
	scraper := monitoring.Scrape(ctx)
	defer scraper.End()

	ctx, cancel := signal.NotifyContext(scraper.Context(), os.Interrupt)
	defer cancel()

	log.Println(strings.Repeat("#", 80))
	log.Println("")
	log.Println("Visit http://localhost:2112/metrics to look at prometheus metrics")
	log.Println("OTEL tracing spans will be printed to stdout periodically")
	log.Println("Press Ctl+C to exit the program")
	log.Println("")
	log.Println(strings.Repeat("#", 80))

	xs := make(chan uint, 100)

	go fibonacci(ctx, 10, xs)
	go printer(ctx, xs)

	select {
	case <-ctx.Done():
		return
	}
}
