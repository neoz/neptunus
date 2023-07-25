package drop

import (
	"github.com/gekatateam/neptunus/core"
	"github.com/gekatateam/neptunus/logger"
	"github.com/gekatateam/neptunus/metrics"
	"github.com/gekatateam/neptunus/plugins"
)

type Drop struct {
	alias string
	pipe  string
	in    <-chan *core.Event
	log   logger.Logger
}

func (p *Drop) Init(_ map[string]any, alias, pipeline string, log logger.Logger) error {
	p.alias = alias
	p.pipe = pipeline
	p.log = log

	return nil
}

func (p *Drop) Prepare(
	in <-chan *core.Event,
	_ chan<- *core.Event,
) {
	p.in = in
}

func (p *Drop) Close() error {
	return nil
}

func (p *Drop) Alias() string {
	return p.alias
}

func (p *Drop) Run() {
	for range p.in {
		metrics.ObserveProcessorSummary("drop", p.alias, p.pipe, metrics.EventAccepted, 0)
		p.log.Debug("event dropped")
		continue
	}
}

func init() {
	plugins.AddProcessor("drop", func() core.Processor {
		return &Drop{}
	})
}
