package starlark

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.starlark.net/lib/json"
	"go.starlark.net/lib/math"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/gekatateam/neptunus/core"
	"github.com/gekatateam/neptunus/metrics"
	"github.com/gekatateam/neptunus/pkg/mapstructure"
	"github.com/gekatateam/neptunus/plugins"
)

type Starlark struct {
	alias string
	pipe  string
	Code  string `mapstructure:"code"`
	File  string `mapstructure:"file"`

	thread *starlark.Thread
	stFunc *starlark.Function

	in  <-chan *core.Event
	out chan<- *core.Event
	log *slog.Logger
}

func (p *Starlark) Init(config map[string]any, alias, pipeline string, log *slog.Logger) error {
	if err := mapstructure.Decode(config, p); err != nil {
		return err
	}

	p.alias = alias
	p.pipe = pipeline
	p.log = log

	if len(p.Code) == 0 && len(p.File) == 0 {
		return errors.New("code or file required")
	}

	if len(p.Code) > 0 {
		p.File = p.alias + ".star"
		goto SCRIPT_LOADED
	}

	if len(p.File) > 0 {
		script, err := os.ReadFile(p.File)
		if err != nil {
			return fmt.Errorf("failed to load %v script: %v", p.File, err)
		}
		p.Code = string(script)
	}
SCRIPT_LOADED:

	builtins := starlark.StringDict{
		"newEvent": starlark.NewBuiltin("newEvent", newEvent),
		"error":    starlark.NewBuiltin("error", newError),
		"struct":   starlark.NewBuiltin("struct", starlarkstruct.Make),
	}

	p.thread = &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) {
			p.log.Debug(fmt.Sprintf("from starlark: %v", msg))
		},
		Load: func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
			switch module {
			case "math.star":
				return starlark.StringDict{
					"math": math.Module,
				}, nil
			case "time.star":
				return starlark.StringDict{
					"time": startime.Module,
				}, nil
			case "json.star":
				return starlark.StringDict{
					"json": json.Module,
				}, nil
			default:
				script, err := os.ReadFile(module)
				if err != nil {
					return nil, err
				}

				entries, err := starlark.ExecFile(thread, module, script, builtins)
				if err != nil {
					return nil, err
				}

				return entries, nil
			}
		},
	}

	_, program, err := starlark.SourceProgram(p.File, p.Code, builtins.Has)
	if err != nil {
		return fmt.Errorf("compilation failed: %v", err)
	}

	globals, err := program.Init(p.thread, builtins)
	if err != nil {
		return fmt.Errorf("initialization failed: %v", err)
	}

	stVal, ok := globals["process"]
	if !ok {
		return errors.New("process(event) function not found in starlark script")
	}

	stFunc, ok := stVal.(*starlark.Function)
	if !ok {
		return errors.New("process is not a function")
	}
	p.stFunc = stFunc

	return nil
}

func (p *Starlark) Prepare(
	in <-chan *core.Event,
	out chan<- *core.Event,
) {
	p.in = in
	p.out = out
}

func (p *Starlark) Close() error {
	return nil
}

func (p *Starlark) Alias() string {
	return p.alias
}

func (p *Starlark) Run() {
	for e := range p.in {
		now := time.Now()
		result, err := starlark.Call(p.thread, p.stFunc, []starlark.Value{&_event{event: e}}, nil)
		if err != nil {
			p.log.Error("exec failed",
				"error", err,
				slog.Group("event",
					"id", e.Id,
					"key", e.RoutingKey,
				),
			)
			e.StackError(fmt.Errorf("exec failed: %v", err))
			e.AddTag("::starlark_processing_failed")
			p.out <- e
			metrics.ObserveProcessorSummary("starlark", p.alias, p.pipe, metrics.EventFailed, time.Since(now))
			continue
		}

		events, err := unpack(result)
		if err != nil {
			p.log.Error("exec failed",
				"error", err,
				slog.Group("event",
					"id", e.Id,
					"key", e.RoutingKey,
				),
			)
			e.StackError(fmt.Errorf("exec failed: %v", err))
			e.AddTag("::starlark_processing_failed")
			p.out <- e
			metrics.ObserveProcessorSummary("starlark", p.alias, p.pipe, metrics.EventFailed, time.Since(now))
			continue
		}

		markAsDone(e, events)
		for _, event := range events {
			p.out <- event
		}
		metrics.ObserveProcessorSummary("starlark", p.alias, p.pipe, metrics.EventAccepted, time.Since(now))
	}
}

func unpack(starValue starlark.Value) ([]*core.Event, error) {
	events := []*core.Event{}

	switch v := starValue.(type) {
	case *_event:
		return append(events, v.event), nil
	case _error:
		return nil, errors.New(v.String())
	case *starlark.List:
		iter := v.Iterate()
		defer iter.Done()
		var value starlark.Value
		for iter.Next(&value) {
			r, err := unpack(value)
			if err != nil {
				return nil, err
			}
			events = append(events, r...)
		}
		return events, nil
	case *starlark.NoneType:
		return events, nil
	}

	return nil, fmt.Errorf("unknown function result, expected event, events list, error or none, got %v", starValue.Type())
}

func markAsDone(e *core.Event, events []*core.Event) {
	for _, v := range events {
		if v == e {
			return
		}
	}
	e.Done()
}

func init() {
	plugins.AddProcessor("starlark", func() core.Processor {
		return &Starlark{}
	})
}
