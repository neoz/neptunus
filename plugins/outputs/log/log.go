package log

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gekatateam/pipeline/core"
	"github.com/gekatateam/pipeline/logger"
	"github.com/gekatateam/pipeline/metrics"
	"github.com/gekatateam/pipeline/pkg/mapstructure"
	"github.com/gekatateam/pipeline/plugins"
)

type Log struct {
	alias string
	Level string `mapstructure:"level"`

	in  <-chan *core.Event
	log logger.Logger
}

func New(config map[string]any, alias string, log logger.Logger) (core.Output, error) {
	l := &Log{
		Level: "info",

		log:   log,
		alias: alias,
	}
	if err := mapstructure.Decode(config, l); err != nil {
		return nil, err
	}

	switch l.Level {
	case "trace", "debug", "info", "warn":
	default:
		return nil, fmt.Errorf("forbidden logging level: %v; expected one of: trace, debug, info, warn", l.Level)
	}

	return l, nil
}

func (o *Log) Init(in <-chan *core.Event) {
	o.in = in
}

func (o *Log) Listen() {
	for e := range o.in {
		now := time.Now()
		event, err := json.Marshal(e)
		if err != nil {
			o.log.Errorf("json marshal failed: %v", err.Error())
			metrics.ObserveOutputSummary("log", o.alias, metrics.EventFailed, time.Since(now))
			continue
		}

		switch o.Level {
		case "trace":
			o.log.Trace(string(event))
		case "debug":
			o.log.Debug(string(event))
		case "info":
			o.log.Info(string(event))
		case "warn":
			o.log.Warn(string(event))
		}
		metrics.ObserveOutputSummary("log", o.alias, metrics.EventAccepted, time.Since(now))
	}
}

func (o *Log) Close() error {
	return nil
}

func init() {
	plugins.AddOutput("log", New)
}
