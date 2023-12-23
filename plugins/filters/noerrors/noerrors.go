package noerrors

import (
	"log/slog"
	"time"

	"github.com/gekatateam/neptunus/core"
	"github.com/gekatateam/neptunus/metrics"
	"github.com/gekatateam/neptunus/plugins"
)

type NoErrors struct {
	alias    string
	pipe     string
	in       <-chan *core.Event
	accepted chan<- *core.Event
	rejected chan<- *core.Event
	log      *slog.Logger
}

func (f *NoErrors) Init(_ map[string]any, alias, pipeline string, log *slog.Logger) error {
	f.alias = alias
	f.pipe = pipeline
	f.log = log

	return nil
}

func (f *NoErrors) SetChannels(
	in <-chan *core.Event,
	rejected chan<- *core.Event,
	accepted chan<- *core.Event,
) {
	f.in = in
	f.rejected = rejected
	f.accepted = accepted

}

func (f *NoErrors) Close() error {
	return nil
}

func (f *NoErrors) Run() {
	for e := range f.in {
		now := time.Now()
		if len(e.Errors) > 0 {
			f.rejected <- e
			metrics.ObserveFliterSummary("noerrors", f.alias, f.pipe, metrics.EventRejected, time.Since(now))
		} else {
			f.accepted <- e
			metrics.ObserveFliterSummary("noerrors", f.alias, f.pipe, metrics.EventAccepted, time.Since(now))
		}
	}
}

func init() {
	plugins.AddFilter("noerrors", func() core.Filter {
		return &NoErrors{}
	})
}
