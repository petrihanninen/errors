package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/petrihanninen/errors/internal/db"
)

const DefaultSystemPrompt = `You are an expert Django developer fixing production errors in the duunitori5 backend.

You have access to tools to read, write, search, and list files in the repository, as well as run commands.

When investigating and fixing errors:
1. Start by understanding the error from the context provided
2. Use the tools to find and read the relevant source code
3. Understand the root cause before making changes
4. Make targeted fixes that address the root cause
5. After making changes, run "make dev-check" to verify there are no linting or type errors
6. Fix any issues that dev-check reports

Important guidelines:
- Focus on resolving the root cause, not just swallowing errors
- Be thorough when investigating - follow the code path from the transaction/request URI
- If you cannot determine the root cause or fix the error, say "CANNOT_FIX" as the very last line of your response
- Err on the side of caution - prefer saying "CANNOT_FIX" rather than doing an incomplete fix
- Keep changes minimal and focused on the error at hand`

func buildErrorContext(eg *db.ErrorGroup, occs []db.ErrorOccurrence) string {
	var sb strings.Builder

	sb.WriteString("## Production Error to Fix\n\n")
	sb.WriteString(fmt.Sprintf("**Error class:** %s\n", eg.Name))
	sb.WriteString(fmt.Sprintf("**Error message:** %s\n", eg.Message))
	sb.WriteString(fmt.Sprintf("**Occurrences:** %d\n", eg.Occurrences))
	sb.WriteString(fmt.Sprintf("**New Relic link:** %s\n\n", eg.Link))

	if len(occs) > 0 {
		sb.WriteString("### Recent Occurrences\n\n")
		for i, o := range occs {
			sb.WriteString(fmt.Sprintf("**Occurrence %d:**\n", i+1))
			occ := map[string]interface{}{
				"error.class":       o.ErrorClass,
				"error.message":     o.Message,
				"host":              o.Host,
				"request.uri":       o.RequestURI,
				"transactionUiName": o.TransactionName,
				"timestamp":         o.OccurredAt,
			}
			data, _ := json.MarshalIndent(occ, "", "  ")
			sb.WriteString("```json\n")
			sb.Write(data)
			sb.WriteString("\n```\n\n")
		}
	}

	sb.WriteString("Please investigate and fix this error. Look at the relevant source code based on the transactionUiName and request.uri fields to understand where the error occurs.\n")

	return sb.String()
}
