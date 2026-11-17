package main

// The Claude brain, through the official Anthropic Go SDK.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// defaultClaudeModel is the model a new Anthropic brain starts on. The page
// lists every model the key can use, so a person picks a cheaper one there.
const defaultClaudeModel = "claude-opus-5-5"

// claudeFallbackModels are the models that take a server-side fallback when a
// safety classifier declines: the request is answered by another model inside
// the same call instead of stopping.
//
//nolint:gochecknoglobals // a closed table.
var claudeFallbackModels = []string{
	"claude-fable-5-2", "claude-fable-5-1", "claude-opus-5-5", "claude-opus-5", "claude-sonnet-5-5",
}

// claudeEffortPrefixes are the model families that take output_config.effort.
// A chat is a light task, so it runs at low effort, which also keeps the pet
// quick to answer.
//
//nolint:gochecknoglobals // a closed table.
var claudeEffortPrefixes = []string{
	"claude-fable-", "claude-mythos-", "claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6",
	"claude-sonnet-5", "claude-sonnet-4-6", "claude-haiku-5",
}

var errClaudeRefused = errors.New("Claude declined to answer this one")

func claudeClient(c completion) anthropic.Client {
	opts := []option.RequestOption{option.WithAPIKey(c.Key)}
	if c.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(c.BaseURL))
	}

	return anthropic.NewClient(opts...)
}

// streamClaude sends one completion to the Messages API and calls onPiece with
// each piece of text as it arrives.
func streamClaude(ctx context.Context, c completion, onPiece func(string)) (string, error) {
	client := claudeClient(c)

	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(c.Model),
		MaxTokens: int64(max(c.MaxTokens, 1024)),
	}

	for _, m := range c.Messages {
		switch m.Role {
		case roleSystem:
			params.System = append(params.System, anthropic.BetaTextBlockParam{Text: m.Content})
		case roleAssistant:
			params.Messages = append(params.Messages, anthropic.BetaMessageParam{OfAssistant: &anthropic.BetaAssistantMessageParam{
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(m.Content)},
			}})
		default:
			params.Messages = append(params.Messages, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(m.Content)))
		}
	}

	if slices.Contains(claudeFallbackModels, c.Model) {
		params.Betas = append(params.Betas, anthropic.AnthropicBetaServerSideFallback2026_07_01)
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
	}

	for _, prefix := range claudeEffortPrefixes {
		if strings.HasPrefix(c.Model, prefix) {
			params.OutputConfig = anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortLow}

			break
		}
	}

	stream := client.Beta.Messages.NewStreaming(ctx, params)
	message := anthropic.BetaMessage{}

	var answer strings.Builder

	for stream.Next() {
		event := stream.Current()
		_ = message.Accumulate(event)

		if delta, ok := event.AsAny().(anthropic.BetaRawContentBlockDeltaEvent); ok {
			if text, ok := delta.Delta.AsAny().(anthropic.BetaTextDelta); ok && text.Text != "" {
				answer.WriteString(text.Text)
				onPiece(text.Text)
			}
		}
	}

	if err := stream.Err(); err != nil {
		return answer.String(), fmt.Errorf("ask Claude: %w", err)
	}

	if message.StopReason == anthropic.BetaStopReasonRefusal {
		return answer.String(), errClaudeRefused
	}

	return answer.String(), nil
}

// claudeModels is every model the key may use, newest first as the API lists them.
func claudeModels(ctx context.Context, c completion) ([]string, error) {
	client := claudeClient(c)
	pager := client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})

	var out []string
	for pager.Next() {
		out = append(out, pager.Current().ID)
	}

	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("list the Claude models: %w", err)
	}

	return out, nil
}
