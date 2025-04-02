package googlechat

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gekatateam/neptunus/core"
	"github.com/gekatateam/neptunus/plugins"
	"github.com/gekatateam/neptunus/plugins/common/batcher"
	"github.com/gekatateam/neptunus/plugins/common/pool"
	"github.com/gekatateam/neptunus/plugins/common/retryer"
	"github.com/gekatateam/neptunus/plugins/common/tls"
)

type GoogleChat struct {
	*core.BaseOutput     `mapstructure:"-"`
	WebhookURL           string        `mapstructure:"webhook_url"`
	Timeout              time.Duration `mapstructure:"timeout"`
	IdleConnTimeout      time.Duration `mapstructure:"idle_conn_timeout"`
	MaxIdleConns         int           `mapstructure:"max_idle_conns"`
	IdleTimeout          time.Duration `mapstructure:"idle_timeout"`
	ThreadMessageField   string        `mapstructure:"thread_message_field"`
	DefaultThreadEnabled bool          `mapstructure:"default_thread_enabled"`

	*tls.TLSClientConfig          `mapstructure:",squash"`
	*batcher.Batcher[*core.Event] `mapstructure:",squash"`
	*retryer.Retryer              `mapstructure:",squash"`

	requestersPool *pool.Pool[*core.Event]
	providedUri    *url.URL
	client         *http.Client
	ser            core.Serializer
}

func (g *GoogleChat) Init() error {
	if len(g.WebhookURL) == 0 {
		return errors.New("webhook_url required")
	}

	if g.DefaultThreadEnabled {
		g.WebhookURL = fmt.Sprintf("%s&messageReplyOption=REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD", g.WebhookURL)
	}
	url, err := url.ParseRequestURI(g.WebhookURL)
	if err != nil {
		return err
	}

	g.providedUri = url

	if g.Batcher.Buffer < 0 {
		g.Batcher.Buffer = 1
	}

	if g.IdleTimeout > 0 && g.IdleTimeout < time.Minute {
		g.IdleTimeout = time.Minute
	}

	tlsConfig, err := g.TLSClientConfig.Config()
	if err != nil {
		return err
	}

	g.client = &http.Client{
		Timeout: g.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
			IdleConnTimeout: g.IdleConnTimeout,
			MaxIdleConns:    g.MaxIdleConns,
		},
	}

	g.requestersPool = pool.New(g.newRequester)

	return nil
}

func (g *GoogleChat) SetSerializer(s core.Serializer) {
	g.ser = s
	// Google Chat webhook uses specific JSON format, so we don't need external serializer
}

func (g *GoogleChat) Run() {
	clearTicker := time.NewTicker(time.Minute)
	if g.IdleTimeout == 0 {
		clearTicker.Stop()
	}

MAIN_LOOP:
	for {
		select {
		case e, ok := <-g.In:
			if !ok {
				clearTicker.Stop()
				break MAIN_LOOP
			}
			g.requestersPool.Get(g.WebhookURL).Push(e)
		case <-clearTicker.C:
			for _, pipeline := range g.requestersPool.Keys() {
				if time.Since(g.requestersPool.Get(pipeline).LastWrite()) > g.IdleTimeout {
					g.requestersPool.Remove(pipeline)
				}
			}
		}
	}
}

func (g *GoogleChat) Close() error {
	g.requestersPool.Close()
	g.client.CloseIdleConnections()
	return nil
}

func (g *GoogleChat) newRequester(uri string) pool.Runner[*core.Event] {
	return &requester{
		BaseOutput:           g.BaseOutput,
		lastWrite:            time.Now(),
		webhookURL:           uri,
		threadMessageField:   g.ThreadMessageField,
		defaultThreadEnabled: g.DefaultThreadEnabled,
		client:               g.client,
		Batcher:              g.Batcher,
		Retryer:              g.Retryer,
		input:                make(chan *core.Event),
		ser:                  g.ser,
	}
}

func init() {
	plugins.AddOutput("google_chat", func() core.Output {
		return &GoogleChat{
			Timeout:         10 * time.Second,
			IdleConnTimeout: 1 * time.Minute,
			MaxIdleConns:    10,
			IdleTimeout:     1 * time.Hour,
			Batcher: &batcher.Batcher[*core.Event]{
				Buffer:   10,
				Interval: 5 * time.Second,
			},
			TLSClientConfig: &tls.TLSClientConfig{},
			Retryer: &retryer.Retryer{
				RetryAttempts: 3,
				RetryAfter:    5 * time.Second,
			},
		}
	})
}
