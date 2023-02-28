package sdk

import (
	"context"
	"fmt"
	"github.com/zerlok/monitoring-go"
	"github.com/zerlok/monitoring-go/scraper"
	"sync"
	"testing"
	"time"
)

const (
	testCaseTimeout = time.Second * 1
)

type testCase struct {
	name              string
	fn                testCaseFunc
	wantOperationsLen uint
	wantMainName      string
}

func (t *testCase) Name(name string) string {
	return fmt.Sprintf("%s/%s", t.name, name)
}

type testCaseFunc func(context.Context, *Sdk)

var testCases = []testCase{
	{
		name: "no operations",
		fn:   func(ctx context.Context, sdk *Sdk) {},
	},
	{
		name: "1 simple main operation",
		fn: func(ctx context.Context, sdk *Sdk) {
			s1 := sdk.ScrapeMainName(ctx, "simple main")
			defer s1.End()
		},
		wantOperationsLen: 1,
		wantMainName:      "simple main",
	},
	{
		name: "1 operation with 1 nested group operation",
		fn: func(ctx context.Context, sdk *Sdk) {
			g := sdk.GroupName("nested")

			s1 := sdk.ScrapeName(ctx, "simple main")
			defer s1.End()

			func(ctx context.Context) {
				sg := g.Scrape(ctx)
				defer sg.End()

				time.Sleep(time.Millisecond * 10)
			}(s1.Context())
		},
		wantOperationsLen: 2,
		wantMainName:      "simple main",
	},
	{
		name: "1 simple operation",
		fn: func(ctx context.Context, sdk *Sdk) {
			s1 := sdk.ScrapeName(ctx, "simple")
			defer s1.End()
		},
		wantOperationsLen: 1,
		wantMainName:      "simple",
	},
	{
		name: "1 operation with 4 nested operations",
		fn: func(ctx context.Context, sdk *Sdk) {
			s1 := sdk.ScrapeName(ctx, "5 ops main")
			defer s1.End()

			func(ctx context.Context) {
				s2 := sdk.Scrape(ctx)
				defer s2.End()

				func(ctx context.Context) {
					s3 := sdk.Scrape(ctx)
					defer s3.End()

					func(ctx context.Context) {
						s4 := sdk.Scrape(ctx)
						defer s4.End()

						func(ctx context.Context) {
							s5 := sdk.Scrape(ctx)
							defer s5.End()
							time.Sleep(time.Millisecond * 10)
						}(s4.Context())
					}(s3.Context())
				}(s2.Context())
			}(s1.Context())
		},
		wantOperationsLen: 5,
		wantMainName:      "5 ops main",
	},
	{
		name: "1 operation with 100 goroutines",
		fn: func(ctx context.Context, sdk *Sdk) {
			s1 := sdk.ScrapeName(ctx, "100 goroutines main")
			defer s1.End()

			amount := 100
			wg := &sync.WaitGroup{}
			wg.Add(amount)

			for i := 0; i < amount; i++ {
				go func(ctx context.Context, wg *sync.WaitGroup) {
					si := sdk.Scrape(ctx)
					defer si.End()
					defer wg.Done()

					time.Sleep(time.Millisecond * 10)
				}(s1.Context(), wg)
			}

			wg.Wait()
		},
		wantOperationsLen: 101,
		wantMainName:      "100 goroutines main",
	},
}

func TestMonitoringCasesMatchExpectations(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.Name("operations count"), func(t *testing.T) {
			ops := runAndGatherOperations(t, tc, testCaseTimeout)
			if opsLen := uint(len(ops)); opsLen != tc.wantOperationsLen {
				t.Errorf("len(ops) = %v, want = %v", opsLen, tc.wantOperationsLen)
			}
		})

		t.Run(tc.Name("operation's main name"), func(t *testing.T) {
			for _, op := range runAndGatherOperations(t, tc, testCaseTimeout) {
				if main := op.Main(); main.Name() != tc.wantMainName {
					t.Errorf("op.Name() = %v, op.Main().Name() = %v, want = %v", op.Name(), main.Name(), tc.wantMainName)
				}
			}
		})
	}
}

func TestMonitoringProperties(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.Name("operation's duration is always greater than 0"), func(t *testing.T) {
			for _, op := range runAndGatherOperations(t, tc, testCaseTimeout) {
				if d := *op.Duration(); d <= 0 {
					t.Errorf("op.Name() = %v, op.Duration() = %v", op.Name(), d)
				}
			}
		})

		t.Run(tc.Name("operation's parent duration is always greater than its own"), func(t *testing.T) {
			for _, op := range runAndGatherOperations(t, tc, testCaseTimeout) {
				if parent := op.Parent(); parent != nil && *parent.Duration() < *op.Duration() {
					t.Errorf("op.Name() = %v, op.Parent().Duration() = %v, op.Duration() = %v", op.Name(), parent.Duration(), op.Duration())
				}
			}
		})

		t.Run(tc.Name("each operation has main"), func(t *testing.T) {
			for _, op := range runAndGatherOperations(t, tc, testCaseTimeout) {
				if main := op.Main(); main == nil {
					t.Errorf("op.Name() = %v, op.Main() shoud not be nil", op.Name())
				}
			}
		})

		t.Run(tc.Name("operation's main duration is always greater than its own"), func(t *testing.T) {
			for _, op := range runAndGatherOperations(t, tc, testCaseTimeout) {
				if main := op.Main(); main != nil && *main.Duration() < *op.Duration() {
					t.Errorf("op.Name() = %v, op.Main().Duration() = %v, op.Duration() = %v", op.Name(), main.Duration(), op.Duration())
				}
			}
		})
	}
}

func mustSetupSdk(ctx context.Context, t *testing.T, tc testCase) (*Sdk, *scraper.InMemoryFactory) {
	sdk, inMemFactory := NewInMemoryOnly(tc.wantOperationsLen + tc.wantOperationsLen/10 + 1)
	if err := sdk.SetupContext(ctx); err != nil {
		t.Fatalf("failed to setup in memory sdk: %v", err.Error())
	}

	return sdk, inMemFactory
}

func runAndGatherOperations(t *testing.T, tc testCase, timeout time.Duration) []monitoring.OperationContext {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	sdk, inMemFactory := mustSetupSdk(ctx, t, tc)
	defer sdk.ShutdownContext(ctx)

	ops := []monitoring.OperationContext{}
	cancel = inMemFactory.GatherEndedOperations(ctx, &ops)
	defer cancel()

	tc.fn(ctx, sdk)

	select {
	case <-ctx.Done():
	}

	return ops
}
