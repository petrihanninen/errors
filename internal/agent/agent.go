package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/petrihanninen/errors/internal/db"
)

const maxTurns = 50

type FixResult struct {
	Success   bool
	CannotFix bool
	Output    string
}

type Agent struct {
	client       *anthropic.Client
	repoDir      string
	systemPrompt string
}

func New(apiKey, repoDir, systemPrompt string) *Agent {
	client := anthropic.NewClient()
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	return &Agent{
		client:       &client,
		repoDir:      repoDir,
		systemPrompt: systemPrompt,
	}
}

func (a *Agent) Fix(ctx context.Context, eg *db.ErrorGroup, occs []db.ErrorOccurrence) (*FixResult, error) {
	errorContext := buildErrorContext(eg, occs)

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(errorContext)),
	}
	tools := toolDefinitions()

	var outputLog strings.Builder

	for turn := 0; turn < maxTurns; turn++ {
		log.Printf("  Agent turn %d/%d", turn+1, maxTurns)

		resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaude4Sonnet20250514,
			MaxTokens: 8192,
			System: []anthropic.TextBlockParam{
				{Text: a.systemPrompt},
			},
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			return nil, fmt.Errorf("anthropic API call: %w", err)
		}

		// Process response blocks
		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range resp.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				outputLog.WriteString(b.Text)
				outputLog.WriteString("\n")
				log.Printf("  [assistant] %s", truncate(b.Text, 200))
			case anthropic.ToolUseBlock:
				inputJSON, _ := json.Marshal(b.Input)
				log.Printf("  [tool_use] %s(%s)", b.Name, truncate(string(inputJSON), 100))

				result, err := executeTool(b.Name, json.RawMessage(inputJSON), a.repoDir)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
				log.Printf("  [tool_result] %s", truncate(result, 200))
				outputLog.WriteString(fmt.Sprintf("\n[Tool: %s] %s\n", b.Name, truncate(result, 500)))

				toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, result, false))
			}
		}

		messages = append(messages, resp.ToParam())

		// If no tool calls, the agent is done
		if len(toolResults) == 0 {
			break
		}

		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	output := outputLog.String()

	// Check for CANNOT_FIX in the last portion of output
	lines := strings.Split(strings.TrimSpace(output), "\n")
	lastLines := lines
	if len(lastLines) > 5 {
		lastLines = lastLines[len(lastLines)-5:]
	}
	for _, line := range lastLines {
		if strings.Contains(line, "CANNOT_FIX") {
			return &FixResult{
				CannotFix: true,
				Output:    output,
			}, nil
		}
	}

	return &FixResult{
		Success: true,
		Output:  output,
	}, nil
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
