package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TrainingExample represents a single training example for fine-tuning
type TrainingExample struct {
	Messages []Message `json:"messages"`
}

// Message represents a single message in a conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Issue structure from the response JSON
type Issue struct {
	Issue    string        `json:"issue"`
	Severity string        `json:"severity"`
	Category string        `json:"category"`
	LogRefs  []interface{} `json:"log_refs,omitempty"`
}

// Response JSON structure
type ResponseData struct {
	Issues []Issue `json:"issues"`
}

func main() {
	var (
		inputPath  string
		outputPath string
		format     string
	)

	// Parse command-line flags
	flag.StringVar(&inputPath, "input", "", "Path to input CSV file or directory of CSV files")
	flag.StringVar(&outputPath, "output", "training_data.jsonl", "Path to output file")
	flag.StringVar(&format, "format", "openai", "Output format (openai, anthropic, llama)")
	flag.Parse()

	if inputPath == "" {
		fmt.Println("Error: input path is required")
		flag.Usage()
		os.Exit(1)
	}

	// Process files
	files := []string{}
	fileInfo, err := os.Stat(inputPath)
	if err != nil {
		fmt.Printf("Error accessing path %s: %v\n", inputPath, err)
		os.Exit(1)
	}

	if fileInfo.IsDir() {
		// Process all CSV files in directory
		entries, err := os.ReadDir(inputPath)
		if err != nil {
			fmt.Printf("Error reading directory %s: %v\n", inputPath, err)
			os.Exit(1)
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".csv") {
				files = append(files, filepath.Join(inputPath, entry.Name()))
			}
		}
	} else {
		// Process single file
		files = append(files, inputPath)
	}

	if len(files) == 0 {
		fmt.Println("No CSV files found to process")
		os.Exit(1)
	}

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer outputFile.Close()

	// Process each file
	var totalExamples int
	fmt.Printf("Processing %d files...\n", len(files))

	for _, file := range files {
		examples, err := processCSVFile(file, format)
		if err != nil {
			fmt.Printf("Error processing file %s: %v\n", file, err)
			continue
		}

		// Write examples to output file
		for _, example := range examples {
			jsonData, err := json.Marshal(example)
			if err != nil {
				fmt.Printf("Error marshaling JSON: %v\n", err)
				continue
			}
			outputFile.WriteString(string(jsonData) + "\n")
		}

		totalExamples += len(examples)
		fmt.Printf("Processed %s: %d examples\n", file, len(examples))
	}

	fmt.Printf("Successfully converted %d examples to %s format in %s\n", totalExamples, format, outputPath)
}

// processCSVFile reads a CSV file and converts it to training examples
func processCSVFile(filePath, format string) ([]TrainingExample, error) {
	// Open the CSV file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create a CSV reader
	reader := csv.NewReader(file)

	// Read the header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Find the column indices
	var systemPromptIdx, promptIdx, responseIdx int
	var foundSystemPrompt, foundPrompt, foundResponse bool

	for i, colName := range header {
		colNameLower := strings.ToLower(colName)
		switch colNameLower {
		case "system_prompt":
			systemPromptIdx = i
			foundSystemPrompt = true
		case "prompt":
			promptIdx = i
			foundPrompt = true
		case "response":
			responseIdx = i
			foundResponse = true
		}
	}

	if !foundPrompt || !foundResponse {
		return nil, fmt.Errorf("CSV must contain 'prompt' and 'response' columns")
	}

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	// Convert records to training examples
	var examples []TrainingExample

	for _, record := range records {
		if len(record) <= max(systemPromptIdx, promptIdx, responseIdx) {
			continue // Skip incomplete records
		}

		prompt := strings.TrimSpace(record[promptIdx])
		response := strings.TrimSpace(record[responseIdx])

		if prompt == "" || response == "" {
			continue // Skip empty prompts or responses
		}

		// Reparse response to check for JSON issues
		var responseData ResponseData
		err := json.Unmarshal([]byte(response), &responseData)
		if err != nil {
			// error, skip
			continue
		}
		for i, issue := range responseData.Issues {
			for j, logRef := range issue.LogRefs {
				if s, ok := logRef.(string); ok {
					v, err := strconv.Atoi(s)
					if err != nil {
						continue
					}
					responseData.Issues[i].LogRefs[j] = v
				}
			}
		}

		data, err := json.Marshal(responseData)
		if err != nil {
			continue
		}
		record[responseIdx] = string(data)

		var example TrainingExample
		// Format according to specified model format
		switch format {
		case "openai":
			example.Messages = formatOpenAI(record, foundSystemPrompt, systemPromptIdx, promptIdx, responseIdx)
		case "anthropic":
			example.Messages = formatAnthropic(record, foundSystemPrompt, systemPromptIdx, promptIdx, responseIdx)
		case "llama":
			example.Messages = formatLlama(record, foundSystemPrompt, systemPromptIdx, promptIdx, responseIdx)
		default:
			example.Messages = formatOpenAI(record, foundSystemPrompt, systemPromptIdx, promptIdx, responseIdx)
		}
		examples = append(examples, example)

	}

	return examples, nil
}

// Format for OpenAI fine-tuning
func formatOpenAI(record []string, hasSystemPrompt bool, systemPromptIdx, promptIdx, responseIdx int) []Message {
	var messages []Message

	if hasSystemPrompt && len(record) > systemPromptIdx && record[systemPromptIdx] != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: strings.TrimSpace(record[systemPromptIdx]),
		})
	}

	messages = append(messages, Message{
		Role:    "user",
		Content: strings.TrimSpace(record[promptIdx]),
	})

	messages = append(messages, Message{
		Role:    "assistant",
		Content: strings.TrimSpace(record[responseIdx]),
	})

	return messages
}

// Format for Anthropic fine-tuning
func formatAnthropic(record []string, hasSystemPrompt bool, systemPromptIdx, promptIdx, responseIdx int) []Message {
	var messages []Message

	if hasSystemPrompt && len(record) > systemPromptIdx && record[systemPromptIdx] != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: strings.TrimSpace(record[systemPromptIdx]),
		})
	}

	messages = append(messages, Message{
		Role:    "human",
		Content: strings.TrimSpace(record[promptIdx]),
	})

	messages = append(messages, Message{
		Role:    "assistant",
		Content: strings.TrimSpace(record[responseIdx]),
	})

	return messages
}

// Format for Llama fine-tuning
func formatLlama(record []string, hasSystemPrompt bool, systemPromptIdx, promptIdx, responseIdx int) []Message {
	var messages []Message

	// For Llama, we typically combine system prompt with user message
	userContent := strings.TrimSpace(record[promptIdx])
	if hasSystemPrompt && len(record) > systemPromptIdx && record[systemPromptIdx] != "" {
		userContent = "<s>[INST] " + strings.TrimSpace(record[systemPromptIdx]) + " [/INST]\n" + userContent
	} else {
		userContent = "<s>[INST] " + userContent + " [/INST]"
	}

	messages = append(messages, Message{
		Role:    "user",
		Content: userContent,
	})

	messages = append(messages, Message{
		Role:    "assistant",
		Content: "</s><s>[INST] " + strings.TrimSpace(record[responseIdx]) + " [/INST]",
	})

	return messages
}

// Helper function to find the maximum value
func max(values ...int) int {
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}
