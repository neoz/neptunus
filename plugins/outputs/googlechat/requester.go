package googlechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gekatateam/neptunus/core"
	"github.com/gekatateam/neptunus/metrics"
	"github.com/gekatateam/neptunus/plugins/common/batcher"
	"github.com/gekatateam/neptunus/plugins/common/retryer"
)

type requester struct {
	*core.BaseOutput

	lastWrite            time.Time
	webhookURL           string
	threadMessageField   string
	defaultThreadEnabled bool

	client *http.Client
	*batcher.Batcher[*core.Event]
	*retryer.Retryer

	input chan *core.Event
	ser   core.Serializer
}

type Thread struct {
	ThreadKey string `json:"threadKey"`
}

// GoogleChatMessage represents a Google Chat webhook message
type GoogleChatMessage struct {
	Text   string `json:"text,omitempty"`
	Thread Thread `json:"thread,omitempty"`
}

func (r *requester) Run() {
	r.Log.Info(fmt.Sprintf("requester for Google Chat webhook spawned"))

	r.Batcher.Run(r.input, func(buf []*core.Event) {
		if len(buf) == 0 {
			return
		}
		now := time.Now()
		r.lastWrite = now

		for _, event := range buf {
			message, threadMessage, threadKey, err := r.createMessage(event)
			if err != nil {
				r.Log.Error("message creation failed",
					"error", err,
					slog.Group("event",
						"id", event.Id,
						"key", event.RoutingKey,
					),
				)
				r.Done <- event
				r.Observe(metrics.EventFailed, time.Since(now))
				continue
			}

			totalBefore := time.Since(now)
			now = time.Now()
			err = r.sendMessage(message, threadKey)
			err = r.sendMessage(threadMessage, threadKey)
			totalAfter := time.Since(now)

			r.Done <- event
			if err != nil {
				r.Log.Error("event processing failed",
					"error", err,
					slog.Group("event",
						"id", event.Id,
						"key", event.RoutingKey,
					),
				)
				r.Observe(metrics.EventFailed, totalBefore+totalAfter)
			} else {
				r.Log.Debug("event processed",
					slog.Group("event",
						"id", event.Id,
						"key", event.RoutingKey,
					),
				)
				r.Observe(metrics.EventAccepted, totalBefore+totalAfter)
			}
		}
	})

	r.Log.Info(fmt.Sprintf("requester for Google Chat webhook closed"))
}

func (r *requester) Push(e *core.Event) {
	r.input <- e
}

func (r *requester) LastWrite() time.Time {
	return r.lastWrite
}

func (r *requester) Close() error {
	close(r.input)
	return nil
}

// createMessage extracts message content and thread key from an event
func (r *requester) createMessage(event *core.Event) (string, string, string, error) {
	data, err := r.ser.Serialize(event)
	if err != nil {
		return "", "", "", fmt.Errorf("googlechat failed to serialize event: %w", err)
	}

	message := string(data)

	// Check if we should append thread message
	var threadMessage string
	if r.threadMessageField != "" {
		if threadMsgField, err := event.GetField(r.threadMessageField); err == nil {
			if threadMsg, ok := threadMsgField.(string); ok {
				threadMessage = threadMsg
			} else {
				threadMessage = fmt.Sprintf("%v", threadMsgField)
			}
		}
	}

	// Get thread key if configured
	var threadKey string
	// If default threading is enabled and no thread key is found, use event ID as thread key
	if r.defaultThreadEnabled {
		// create random uuid
		threadKey = uuid.New().String()
		//threadKey = event.Id
	}

	return message, threadMessage, threadKey, nil
}

// sendMessage sends a message to Google Chat webhook
func (r *requester) sendMessage(message, threadKey string) error {
	return r.Retryer.Do("send message to Google Chat", r.Log, func() error {
		chatMsg := GoogleChatMessage{
			Text: message,
		}

		// Add thread key if available
		if threadKey != "" {
			chatMsg.Thread.ThreadKey = threadKey
		}

		payload, err := json.Marshal(chatMsg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		req, err := http.NewRequest("POST", r.webhookURL, bytes.NewReader(payload))
		if err != nil {
			return err
		}

		req.Header.Add("Content-Type", "application/json; charset=UTF-8")

		res, err := r.client.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			bodyBytes, _ := io.ReadAll(res.Body)
			return fmt.Errorf("received non-success status code: %d, body: %s", res.StatusCode, string(bodyBytes))
		}

		return nil
	})
}
