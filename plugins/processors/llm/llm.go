package llm

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/gekatateam/neptunus/core"
	"github.com/gekatateam/neptunus/metrics"
	"github.com/gekatateam/neptunus/plugins"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
	"golang.org/x/exp/slog"
)

// LLM processor integrates with LLM for language model processing
type LLM struct {
	*core.BaseProcessor `mapstructure:"-"`
	// Engine LLM to use, support ollama, google, openai
	LLMType string `mapstructure:"llm_type"`
	// BaseURL for the LLM API
	BaseURL string `mapstructure:"base_url"`
	// Model to use for generation
	Model string `mapstructure:"model"`
	// PromptFrom field name to get prompt from
	PromptFrom string `mapstructure:"prompt_from"`
	// ResponseTo field name to store response
	ResponseTo string `mapstructure:"response_to"`
	// Temperature controls randomness (0.0-1.0)
	Temperature float64 `mapstructure:"temperature"`

	TopK int `mapstructure:"top_k"`
	//
	TopP float64 `mapstructure:"top_p"`

	// MaxTokens maximum number of tokens to generate
	MaxTokens int `mapstructure:"max_tokens"`
	// TimeoutSeconds for request
	TimeoutSeconds int `mapstructure:"timeout_seconds"`
	// SystemPrompt to use as context
	SystemPrompt string `mapstructure:"system_prompt"`
	// JSONMode to enable JSON output
	JSONMode bool `mapstructure:"json_mode"`
	// Get data from raw output JSON key to output
	JSONModeGetKey string `mapstructure:"json_mode_get_key"`
	// KeepAlive controls how long the model will stay loaded into memory following the request
	KeepAlive string `mapstructure:"keep_alive"`
	// api_key for llm
	ApiKey string `mapstructure:"api_key"`
	// save_csv_path for llm
	SaveCSVPath string `mapstructure:"save_csv_path"`
	// max file size for csv to rotate
	MaxFileSize int64 `mapstructure:"max_csv_size"`
	// ollama client
	client llms.Model
}

// SaveCSV saves the LLM prompt, response, and metadata to a CSV file
func (p *LLM) SaveCSV(systemprompt, prompt, response, model string) error {
	if p.SaveCSVPath == "" {
		p.Log.Debug("SaveCSV called but save_csv_path not configured, skipping")
		return nil
	}

	// Get directory and base filename
	dir := filepath.Dir(p.SaveCSVPath)
	baseFileName := filepath.Base(p.SaveCSVPath)
	ext := filepath.Ext(baseFileName)
	nameWithoutExt := strings.TrimSuffix(baseFileName, ext)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Check if file exists and its size
	currentPath := p.SaveCSVPath
	suffix := 0
	for {
		fileInfo, err := os.Stat(currentPath)
		// If file doesn't exist or is small enough, use this path
		if os.IsNotExist(err) || (err == nil && fileInfo.Size() < p.MaxFileSize) {
			break
		}

		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to get file info: %w", err)
		}

		// If file exists and is too large, increment suffix
		currentPath = filepath.Join(dir, fmt.Sprintf("%s.%d%s", nameWithoutExt, suffix, ext))
		suffix++
	}
	if currentPath != p.SaveCSVPath {
		p.Log.Debug("rotating CSV file",
			"old_path", p.SaveCSVPath,
			"new_path", currentPath,
		)
		err := os.Rename(p.SaveCSVPath, currentPath)
		if err != nil {
			return err
		}
	}

	// Open file in append mode or create if doesn't exist
	file, err := os.OpenFile(p.SaveCSVPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	// Create CSV writer
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Check if file is empty to write header
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// If file is empty, write header
	if fileInfo.Size() == 0 {
		header := []string{"timestamp", "model", "system_prompt", "prompt", "response", "llm_type"}
		if err := writer.Write(header); err != nil {
			return fmt.Errorf("failed to write CSV header: %w", err)
		}
	}

	// Write the record
	record := []string{
		time.Now().Format(time.RFC3339),
		p.Model,
		strings.TrimSpace(systemprompt),
		strings.TrimSpace(prompt),
		strings.TrimSpace(response),
		p.LLMType,
	}

	if err := writer.Write(record); err != nil {
		return fmt.Errorf("failed to write CSV record: %w", err)
	}

	p.Log.Debug("saved LLM interaction to CSV",
		"path", p.SaveCSVPath,
		"model", p.Model,
	)

	return nil
}

// Start begins the LLM processor
func (p *LLM) Init() error {
	if p.LLMType == "" {
		return fmt.Errorf("llm_type is required, support ollama, google, openai")
	}
	if p.Model == "" {
		return fmt.Errorf("model is required")
	}

	if p.PromptFrom == "" {
		return fmt.Errorf("prompt_from field is required")
	}

	if p.ResponseTo == "" {
		p.ResponseTo = "llm.output"
	}

	if p.SystemPrompt == "" {
		return fmt.Errorf("system_prompt field is required")
	}

	if p.MaxTokens == 0 {
		p.MaxTokens = 4096
	}

	// Set defaults
	if p.BaseURL == "" {
		p.BaseURL = "http://localhost:11434"
	}

	if p.TimeoutSeconds == 0 {
		p.TimeoutSeconds = 30
	}

	if p.Temperature == 0 {
		p.Temperature = 0.5
	}

	if p.KeepAlive == "" {
		p.KeepAlive = "5m"
	}

	if p.MaxFileSize == 0 {
		p.MaxFileSize = 50 * 1024 * 1024
	}

	// Initialize LLM client
	var client llms.Model
	var err error
	switch p.LLMType {
	case "ollama":
		client, err = ollama.New(
			ollama.WithServerURL(p.BaseURL),
			ollama.WithModel(p.Model),
			ollama.WithKeepAlive(p.KeepAlive),
			ollama.WithRunnerNumCtx(p.MaxTokens),
		)
		break
	case "google":
		client, err = googleai.New(context.Background(),
			googleai.WithAPIKey(p.ApiKey),
			googleai.WithDefaultModel(p.Model),
			googleai.WithDefaultMaxTokens(p.MaxTokens),
			googleai.WithDefaultTemperature(p.Temperature),
			googleai.WithDefaultEmbeddingModel("embedding-001"),
			googleai.WithDefaultCandidateCount(1),
			googleai.WithDefaultTopK(3),
			googleai.WithDefaultTopP(0.95),
			googleai.WithHarmThreshold(googleai.HarmBlockNone),
		)
		break
	case "openai":
		client, err = openai.New(
			openai.WithBaseURL(p.BaseURL),
			openai.WithModel(p.Model),
			openai.WithToken(p.ApiKey),
		)
	}
	if err != nil {
		return fmt.Errorf("failed to initialize LLM client: %w", err)
	}
	p.client = client

	return nil
}

// Stop the processor
func (p *LLM) Close() error {
	return nil
}

func FlattenMap(m map[string]interface{}) map[string]interface{} {
	flat := make(map[string]interface{})
	for k, v := range m {
		if vm, ok := v.(map[string]interface{}); ok {
			for kk, vv := range FlattenMap(vm) {
				flat[k+"."+kk] = vv
			}
		} else {
			flat[k] = v
		}
	}
	return flat
}

func GetJSONData(s string) (map[string]interface{}, error) {
	data := strings.TrimPrefix(s, "```json")
	data = strings.TrimSuffix(data, "```")
	var i map[string]interface{}
	err := json.Unmarshal([]byte(data), &i)
	if err != nil {
		return nil, err
	}
	return i, nil
}

// Process events from input channel
func (p *LLM) Run() {
	for e := range p.In {
		now := time.Now()

		// Get prompt from event
		fieldValue, err := e.GetField(p.PromptFrom)
		if err != nil {
			p.handleError(e, now, fmt.Errorf("failed to get prompt field '%s': %w", p.PromptFrom, err))
			continue
		}

		prompt, err := toString(fieldValue)
		if err != nil {
			p.handleError(e, now, fmt.Errorf("failed to convert prompt field to string: %w", err))
			continue
		}

		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			p.handleError(e, now, fmt.Errorf("prompt is empty"))
			continue
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.TimeoutSeconds)*time.Second)

		// Prepare generation options
		var opts []llms.CallOption
		if p.MaxTokens > 0 {
			opts = append(opts, llms.WithMaxTokens(p.MaxTokens))
		}
		if p.JSONMode {
			opts = append(opts, llms.WithJSONMode())
		}

		//if p.TopP != 0.0 {
		//	opts = append(opts, llms.WithTopP(p.TopP))
		//}
		//
		//if p.TopK != 0 {
		//	opts = append(opts, llms.WithTopK(p.TopK))
		//}

		content := []llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeSystem, p.SystemPrompt),
			llms.TextParts(llms.ChatMessageTypeHuman, prompt),
		}

		// Call LLM
		completion, err := p.client.GenerateContent(ctx, content,
			opts...,
		)
		cancel()
		if err != nil {
			p.handleError(e, now, fmt.Errorf("LLM API call failed: %w", err))
			continue
		}

		if completion == nil || len(completion.Choices) == 0 {
			p.handleError(e, now, fmt.Errorf("no completion data"))
			continue
		}

		completion.Choices[0].Content = strings.TrimSpace(completion.Choices[0].Content)

		e.SetLabel("SystemPrompt", p.SystemPrompt)
		e.SetLabel("UserPrompt", prompt)
		e.SetLabel("Response", completion.Choices[0].Content)
		info, err := toString(completion.Choices[0].GenerationInfo)
		if err == nil {
			e.SetLabel("GenerationInfo", info)
		}

		if p.JSONMode {
			data := strings.TrimPrefix(completion.Choices[0].Content, "```json")
			data = strings.TrimSuffix(data, "```")

			if p.JSONModeGetKey != "" {
				it, err := GetJSONData(data)
				if err != nil {
					p.handleError(e, now, fmt.Errorf("llm failed to parse ouput JSON data: %w", err))
					continue
				}
				if out, ok := it[p.JSONModeGetKey]; ok {
					bData, err := json.Marshal(out)
					if err != nil {
						p.handleError(e, now, fmt.Errorf("llm failed to marshal ouput JSON value '%s': %w", p.ResponseTo, err))
						continue
					}
					if err := e.SetField(p.ResponseTo, bData); err != nil {
						p.handleError(e, now, fmt.Errorf("llm failed to set response field '%s': %w", p.ResponseTo, err))
						continue
					}
				} else {
					p.Log.Warn("llm No key found in JSON data, return {}",
						slog.Group("event",
							"id", e.Id,
							"key", e.RoutingKey,
						),
					)

					if err := e.SetField(p.ResponseTo, "{}"); err != nil {
						p.Log.Error("llm failed to set response field",
							"error", err,
							slog.Group("event",
								"id", e.Id,
								"key", e.RoutingKey,
							),
						)
					}
					p.handleError(e, now, fmt.Errorf("llm failed to get JSON key %s", p.JSONModeGetKey))
					continue
				}
			} else {
				if err := e.SetField(p.ResponseTo, data); err != nil {
					p.handleError(e, now, fmt.Errorf("llm failed to set response field '%s': %w", p.ResponseTo, err))
					continue
				}
			}
		} else {
			// Set response in event
			if err := e.SetField(p.ResponseTo, completion.Choices[0].Content); err != nil {
				p.handleError(e, now, fmt.Errorf("failed to set response field '%s': %w", p.ResponseTo, err))
				continue
			}
		}

		p.Log.Debug("event processed",
			slog.Group("event",
				"id", e.Id,
				"key", e.RoutingKey,
			),
		)
		p.Out <- e
		p.Observe(metrics.EventAccepted, time.Since(now))

		if p.SaveCSVPath != "" {
			if err := p.SaveCSV(p.SystemPrompt, prompt, completion.Choices[0].Content, p.Model); err != nil {
				p.Log.Error("save CSV failed",
					"error", err,
				)
			}
		}
	}
}

// handleError processes errors in a uniform way
func (p *LLM) handleError(e *core.Event, startTime time.Time, err error) {
	p.Log.Error("LLM processor error",
		"error", err,
		slog.Group("event",
			"id", e.Id,
			"key", e.RoutingKey,
		),
	)
	e.StackError(err)
	p.Out <- e
	p.Observe(metrics.EventFailed, time.Since(startTime))
}

// Observe metrics
func (p *LLM) Observe(status metrics.EventStatus, duration time.Duration) {
	// Implement metrics observation based on your metrics package
	// This is a placeholder
}

// Helper function to convert any value to string
func toString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case fmt.Stringer:
		return v.String(), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// Register the processor
func init() {
	plugins.AddProcessor("llm", func() core.Processor {
		return &LLM{
			TimeoutSeconds: 30,
		}
	})
}
