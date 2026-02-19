#!/usr/bin/env bash
set -euo pipefail

ERRORS_FILE="$(cd "$(dirname "$0")" && pwd)/ERRORS.json"
REPO_DIR="$(cd "$(dirname "$0")/duunitori5" && pwd)"
BASE_BRANCH="testing"

if ! command -v jq &>/dev/null; then
  echo "Error: jq is required. Install with: brew install jq"
  exit 1
fi

if ! command -v claude &>/dev/null; then
  echo "Error: claude CLI is required."
  exit 1
fi

if [[ ! -f "$ERRORS_FILE" ]]; then
  echo "Error: $ERRORS_FILE not found"
  exit 1
fi

# Count remaining todo errors
todo_count=$(jq '[.[] | select(.status == "todo")] | length' "$ERRORS_FILE")
echo "Found $todo_count todo error(s) in ERRORS.json"

if [[ "$todo_count" -eq 0 ]]; then
  echo "No todo errors left. Done!"
  exit 0
fi

error_count=$(jq '. | length' "$ERRORS_FILE")

for i in $(seq 0 $((error_count - 1))); do
  # Re-read status each iteration (file may have changed)
  status=$(jq -r ".[$i].status" "$ERRORS_FILE")

  if [[ "$status" != "todo" ]]; then
    continue
  fi

  # Check if any todos remain
  todo_remaining=$(jq '[.[] | select(.status == "todo")] | length' "$ERRORS_FILE")
  if [[ "$todo_remaining" -eq 0 ]]; then
    echo "No more todo errors. Done!"
    break
  fi

  # Extract error details
  error_name=$(jq -r ".[$i].name" "$ERRORS_FILE")
  error_message=$(jq -r ".[$i].message" "$ERRORS_FILE")
  error_json=$(jq -c ".[$i]" "$ERRORS_FILE")

  echo ""
  echo "============================================"
  echo "Processing error $((i + 1))/$error_count"
  echo "  Name:    $error_name"
  echo "  Message: $error_message"
  echo "============================================"

  # Mark as doing
  jq ".[$i].status = \"doing\"" "$ERRORS_FILE" > "$ERRORS_FILE.tmp" && mv "$ERRORS_FILE.tmp" "$ERRORS_FILE"

  # Create a branch name from the error name (sanitize for git)
  branch_suffix=$(echo "${error_name}--${error_message}" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | head -c 60 | sed 's/-$//')
  branch_name="bugfix/error-${branch_suffix}"

  # Ensure we're on base branch and up to date
  cd "$REPO_DIR"
  git checkout "$BASE_BRANCH"
  git pull --ff-only origin "$BASE_BRANCH" 2>/dev/null || true

  # Create and switch to new branch
  git checkout -b "$branch_name"

  echo "Created branch: $branch_name"

  # Build the prompt for claude
  prompt=$(cat <<PROMPT_EOF
You are fixing a production error in the duunitori5 Django backend.

Here is the error from New Relic:

$error_json

Error class: $error_name
Error message: $error_message

Please investigate and fix this error. Look at the relevant source code based on the transactionUiName and request.uri fields to understand where the error occurs.

Be thorough and detailed when ivestigating the error. Focus on resolving the root cause, not just swallowing errors. If you cannot determine the root cause or fix the error, say "CANNOT_FIX" as the very last line of your response. Err on the side of caution and prefer saying "CANNOT_FIX" rather than doing an incomplete fix.

After making your changes, run dev-check to ensure there are no linting or type errors: make dev-check

Fix any issues that dev-check reports before finishing.

PROMPT_EOF
)

  # Run claude and capture output
  echo "Running claude to fix the error..."
  claude_output=$(claude --print --dangerously-skip-permissions -p "$prompt" 2>&1) || true

  # Check if claude gave up
  last_line=$(echo "$claude_output" | tail -n 5)
  if echo "$last_line" | grep -q "CANNOT_FIX"; then
    echo "Claude could not fix this error. Marking as failed."
    # Return to base branch, delete the fix branch
    git checkout "$BASE_BRANCH"
    git branch -D "$branch_name"
    # Mark as failed
    jq ".[$i].status = \"failed\"" "$ERRORS_FILE" > "$ERRORS_FILE.tmp" && mv "$ERRORS_FILE.tmp" "$ERRORS_FILE"
    continue
  fi

  # Check if there are any changes to commit
  cd "$REPO_DIR"
  if git diff --quiet && git diff --cached --quiet; then
    echo "No changes were made. Marking as failed."
    git checkout "$BASE_BRANCH"
    git branch -D "$branch_name"
    jq ".[$i].status = \"failed\"" "$ERRORS_FILE" > "$ERRORS_FILE.tmp" && mv "$ERRORS_FILE.tmp" "$ERRORS_FILE"
    continue
  fi

  # Commit changes
  echo "Committing changes..."
  git add -A
  git commit -m "$(cat <<EOF
Fix production error: $error_name

$error_message

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"

  echo "Changes committed on branch: $branch_name"

  # Mark as done
  jq ".[$i].status = \"done\"" "$ERRORS_FILE" > "$ERRORS_FILE.tmp" && mv "$ERRORS_FILE.tmp" "$ERRORS_FILE"

  # Return to base branch
  git checkout "$BASE_BRANCH"

  echo "Error $((i + 1)) done!"
done

echo ""
echo "============================================"
echo "Summary:"
echo "  Done:   $(jq '[.[] | select(.status == "done")] | length' "$ERRORS_FILE")"
echo "  Failed: $(jq '[.[] | select(.status == "failed")] | length' "$ERRORS_FILE")"
echo "  Todo:   $(jq '[.[] | select(.status == "todo")] | length' "$ERRORS_FILE")"
echo "============================================"
