package http

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gekatateam/neptunus/core"
	"github.com/gekatateam/neptunus/logger"
	"github.com/gekatateam/neptunus/metrics"
	"github.com/gekatateam/neptunus/pkg/mapstructure"
	"github.com/gekatateam/neptunus/plugins"
)

type Http struct {
	alias        string
	pipe         string
	Address      string        `mapstructure:"address"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`

	server   *http.Server
	listener net.Listener

	log logger.Logger
	out chan<- *core.Event
}

func New(config map[string]any, alias, pipeline string, log logger.Logger) (core.Input, error) {
	h := &Http{
		Address:      ":9800",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,

		log:   log,
		alias: alias,
		pipe:  pipeline,
	}
	if err := mapstructure.Decode(config, h); err != nil {
		return nil, err
	}

	if len(h.Address) == 0 {
		return nil, errors.New("address required")
	}

	listener, err := net.Listen("tcp", h.Address); if err != nil {
		return nil, fmt.Errorf("error creating listener: %v", err)
	}

	h.listener = listener
	mux := http.NewServeMux()
	mux.Handle("/", h)
	h.server = &http.Server{
		ReadTimeout:  h.ReadTimeout,
		WriteTimeout: h.WriteTimeout,
		Handler:      mux,
	}

	return h, nil
}

func (i *Http) Init(out chan<- *core.Event) {
	i.out = out
}

func (i *Http) Serve() {
	i.log.Infof("starting http server on %v", i.Address)
	if err := i.server.Serve(i.listener); err != nil && err != http.ErrServerClosed {
		i.log.Errorf("http server startup failed: %v", err.Error())
	} else {
		i.log.Debug("http server stopped")
	}
}

func (i *Http) Close() error {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()
	i.server.SetKeepAlivesEnabled(false)
	if err := i.server.Shutdown(ctx); err != nil {
		i.log.Errorf("http server graceful shutdown ends with error: %v", err.Error())
	}
	return nil
}

func (i *Http) Alias() string {
	return i.alias
}

func (i *Http) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
	i.log.Debugf("received request from: %v", r.RemoteAddr)

	var cursor = 0
	scanner := bufio.NewScanner(r.Body)
	for scanner.Scan() {
		now := time.Now()
		cursor++

		if err := scanner.Err(); err != nil {
			errMsg := fmt.Sprintf("reading error at line %v: %v", cursor, err.Error())
			i.log.Errorf(errMsg)
			http.Error(w, errMsg, http.StatusInternalServerError)
			metrics.ObserveInputSummary("http", i.alias, i.pipe, metrics.EventFailed, time.Since(now))
			return
		}

		e := core.NewEvent(r.URL.Path)
		err := json.Unmarshal(scanner.Bytes(), &e.Data)
		if err != nil {
			errMsg := fmt.Sprintf("bad json at line %v: %v", cursor, err.Error())
			i.log.Errorf(errMsg)
			http.Error(w, errMsg, http.StatusBadRequest)
			metrics.ObserveInputSummary("http", i.alias, i.pipe, metrics.EventFailed, time.Since(now))
			return
		}

		e.Labels["input"] = "http"
		e.Labels["server"] = i.Address
		e.Labels["sender"] = r.RemoteAddr
		i.out <- e
		metrics.ObserveInputSummary("http", i.alias, i.pipe, metrics.EventAccepted, time.Since(now))
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("accepted events: %v\n", cursor)))
}

func init() {
	plugins.AddInput("http", New)
}
