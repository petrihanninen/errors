package agent

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/petrihanninen/errors/internal/db"
)

const DefaultSystemPrompt = `You are fixing a production error in the duunitori5 Django backend.

Please investigate and fix this error. Look at the relevant source code based on the transactionUiName and request.uri fields to understand where the error occurs.

Be thorough and detailed when investigating the error. Focus on resolving the root cause, not just swallowing errors. If you cannot determine the root cause or fix the error, say "CANNOT_FIX" as the very last line of your response. Err on the side of caution and prefer saying "CANNOT_FIX" rather than doing an incomplete fix.

After making your changes, run dev-check to ensure there are no linting or type errors: make dev-check

Fix any issues that dev-check reports before finishing.`

type FixResult struct {
	Success   bool
	CannotFix bool
	Output    string
}

func Fix(repoDir, systemPrompt string, eg *db.ErrorGroup, occs []db.ErrorOccurrence) (*FixResult, error) {
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	prompt := buildPrompt(systemPrompt, eg, occs)

	cmd := exec.Command("claude", "--print", "--dangerously-skip-permissions", "-p", prompt)
	cmd.Dir = repoDir

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		return &FixResult{
			Output: outputStr,
		}, fmt.Errorf("claude CLI: %w\n%s", err, outputStr)
	}

	// Check for CANNOT_FIX in the last few lines
	lines := strings.Split(strings.TrimSpace(outputStr), "\n")
	lastLines := lines
	if len(lastLines) > 5 {
		lastLines = lastLines[len(lastLines)-5:]
	}
	for _, line := range lastLines {
		if strings.Contains(line, "CANNOT_FIX") {
			return &FixResult{
				CannotFix: true,
				Output:    outputStr,
			}, nil
		}
	}

	return &FixResult{
		Success: true,
		Output:  outputStr,
	}, nil
}

func buildPrompt(systemPrompt string, eg *db.ErrorGroup, occs []db.ErrorOccurrence) string {
	var sb strings.Builder

	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("Error class: %s\n", eg.Name))
	sb.WriteString(fmt.Sprintf("Error message: %s\n", eg.Message))
	sb.WriteString(fmt.Sprintf("Occurrences: %d\n", eg.Occurrences))
	sb.WriteString(fmt.Sprintf("New Relic link: %s\n\n", eg.Link))

	if len(occs) > 0 {
		sb.WriteString("Recent occurrences:\n\n")
		for i, o := range occs {
			occ := map[string]interface{}{
				"error.class":       o.ErrorClass,
				"error.message":     o.Message,
				"host":              o.Host,
				"request.uri":       o.RequestURI,
				"transactionUiName": o.TransactionName,
				"timestamp":         o.OccurredAt,
			}
			data, _ := json.MarshalIndent(occ, "", "  ")
			sb.WriteString(fmt.Sprintf("Occurrence %d:\n%s\n\n", i+1, string(data)))
		}
	}

	return sb.String()
}
