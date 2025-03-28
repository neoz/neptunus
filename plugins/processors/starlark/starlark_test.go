package starlark_test

import (
	"sync"
	"testing"

	"github.com/gekatateam/neptunus/core"
	"github.com/gekatateam/neptunus/logger"
	"github.com/gekatateam/neptunus/metrics"
	"github.com/gekatateam/neptunus/pkg/mapstructure"
	common "github.com/gekatateam/neptunus/plugins/common/starlark"
	"github.com/gekatateam/neptunus/plugins/processors/starlark"
)

func TestStarlark(t *testing.T) {
	tests := map[string]*struct {
		config map[string]any
		input  chan *core.Event
		output chan *core.Event
		drop   chan *core.Event
		event  *core.Event
	}{
		"return-same-event": {
			config: map[string]any{
				"code": `
def process(event):
	return event
				`,
			},
			input:  make(chan *core.Event, 100),
			output: make(chan *core.Event, 100),
			drop:   make(chan *core.Event, 100),
			event:  core.NewEvent("test-key"),
		},
		"return-none": {
			config: map[string]any{
				"code": `
def process(event):
	return None
				`,
			},
			input:  make(chan *core.Event, 100),
			output: make(chan *core.Event, 100),
			drop:   make(chan *core.Event, 100),
			event:  core.NewEvent("test-key"),
		},
		"return-error": {
			config: map[string]any{
				"code": `
def process(event):
	return error("that was really bad")
				`,
			},
			input:  make(chan *core.Event, 100),
			output: make(chan *core.Event, 100),
			drop:   make(chan *core.Event, 100),
			event:  core.NewEvent("test-key"),
		},
		"return-new-event": {
			config: map[string]any{
				"code": `
def process(event):
	return newEvent("super-test-key")
				`,
			},
			input:  make(chan *core.Event, 100),
			output: make(chan *core.Event, 100),
			drop:   make(chan *core.Event, 100),
			event:  core.NewEvent("test-key"),
		},
		"return-list-of-new": {
			config: map[string]any{
				"code": `
def process(event):
	return [newEvent("new")]
				`,
			},
			input:  make(chan *core.Event, 100),
			output: make(chan *core.Event, 100),
			drop:   make(chan *core.Event, 100),
			event:  core.NewEvent("test-key"),
		},
		"return-list-of-same": {
			config: map[string]any{
				"code": `
def process(event):
	return [event]
				`,
			},
			input:  make(chan *core.Event, 100),
			output: make(chan *core.Event, 100),
			drop:   make(chan *core.Event, 100),
			event:  core.NewEvent("test-key"),
		},
		"return-list-of-mixed": {
			config: map[string]any{
				"code": `
def process(event):
	return [event, newEvent("new-test-key")]
				`,
			},
			input:  make(chan *core.Event, 100),
			output: make(chan *core.Event, 100),
			drop:   make(chan *core.Event, 100),
			event:  core.NewEvent("test-key"),
		},
		"return-bad-type": {
			config: map[string]any{
				"code": `
def process(event):
	return "event"
				`,
			},
			input:  make(chan *core.Event, 100),
			output: make(chan *core.Event, 100),
			drop:   make(chan *core.Event, 100),
			event:  core.NewEvent("test-key"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			processor := &starlark.Starlark{
				Starlark: &common.Starlark{},
				BaseProcessor: &core.BaseProcessor{
					Log: logger.Mock(),
					Obs: metrics.ObserveMock,
				},
			}
			if err := mapstructure.Decode(test.config, processor); err != nil {
				t.Fatalf("processor config not applied: %v", err)
			}
			if err := processor.Init(); err != nil {
				t.Fatalf("processor not initialized: %v", err)
			}

			wg := &sync.WaitGroup{}
			processor.SetChannels(test.input, test.output, test.drop)
			wg.Add(1)
			go func() {
				processor.Run()
				wg.Done()
			}()

			test.event.AddHook(func() {})
			test.input <- test.event
			close(test.input)
			processor.Close()
			wg.Wait()
			close(test.output)
			close(test.drop)

			for e := range test.drop {
				e.Done()
			}

			duty := 0
			for e := range test.output {
				e.Done()
				duty = int(e.Duty())
			}

			if test.event.Duty() > 0 {
				t.Fatal("incoming event not delivered")
			}

			if duty > 0 {
				t.Fatal("outgoing event not delivered")
			}
		})
	}
}
