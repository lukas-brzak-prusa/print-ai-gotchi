package main

// One client for every brain: the local llama-server, Ollama, and any cloud
// service all speak the OpenAI chat completions API, so the pet asks them the
// same way and reads the answer as a stream of small pieces.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Message is one turn of a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const (
	roleSystem    = "system"
	roleUser      = "user"
	roleAssistant = "assistant"
)

// completion is what one request asks for.
type completion struct {
	BaseURL   string
	Model     string
	Key       string
	Messages  []Message
	MaxTokens int
	// Temperature is low for facts and higher for a joke.
	Temperature float64
	// Client reaches the brain when it needs a client of its own, as the
	// local server on its unix socket does. Nil uses the shared one.
	Client *http.Client
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// stream sends one completion and calls onPiece with each piece of the answer
// as it arrives. It returns the whole answer.
func stream(ctx context.Context, client *http.Client, c completion, onPiece func(string)) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.Model, Messages: c.Messages, Stream: true, MaxTokens: c.MaxTokens, Temperature: c.Temperature,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}

	if c.Client != nil {
		client = c.Client
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("reach the brain: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

		return "", fmt.Errorf("the brain answered %s: %s", resp.Status, strings.TrimSpace(string(detail)))
	}

	var answer strings.Builder

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}

		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}

		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Error != nil {
			return answer.String(), errors.New(chunk.Error.Message)
		}

		for _, choice := range chunk.Choices {
			if piece := choice.Delta.Content; piece != "" {
				answer.WriteString(piece)
				onPiece(piece)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return answer.String(), fmt.Errorf("read the answer: %w", err)
	}

	return answer.String(), nil
}

// openAIModels is every model the server lists at /models.
func openAIModels(ctx context.Context, client *http.Client, c completion) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}

	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", req.URL.Host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 300))

		return nil, fmt.Errorf("%s answered %s: %s", req.URL.Host, resp.Status, strings.TrimSpace(string(detail)))
	}

	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&listing); err != nil {
		return nil, fmt.Errorf("read the model list: %w", err)
	}

	out := make([]string, 0, len(listing.Data))
	for _, m := range listing.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}

	return out, nil
}
