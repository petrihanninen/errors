#!/usr/bin/env bash
set -euo pipefail

# Load environment variables
source "$HOME/.env"

ENTITY_GUID="NTIyMTg2fEFQTXxBUFBMSUNBVElPTnw1MjQyNTgxMjE"
ACCOUNT_ID="$NEW_RELIC_ACCOUNT_ID"
API_KEY="$NEW_RELIC_API_KEY"
JSON_FILE="ERRORS.json"
TIME_RANGE="${1:-24 hours}"

# Convert time range to seconds
case "$TIME_RANGE" in
  *hour*) SECONDS_AGO=$(( ${TIME_RANGE%%[^0-9]*} * 3600 )) ;;
  *day*)  SECONDS_AGO=$(( ${TIME_RANGE%%[^0-9]*} * 86400 )) ;;
  *min*)  SECONDS_AGO=$(( ${TIME_RANGE%%[^0-9]*} * 60 )) ;;
  *)      SECONDS_AGO=86400 ;;
esac

NOW=$(date +%s)000
START=$(( $(date +%s) - SECONDS_AGO ))000
SINCE_NRQL="${TIME_RANGE%%[^0-9]*} ${TIME_RANGE##*[0-9] }"

echo "Fetching unresolved errors from the last ${TIME_RANGE}..."

# --- Step 1: Fetch error groups ---
CURSOR=""
ALL_GROUPS="[]"

while true; do
  CURSOR_ARG=""
  if [ -n "$CURSOR" ]; then
    CURSOR_ARG=", cursor: \"$CURSOR\""
  fi

  GQL="{ actor { errorsInbox { errorGroups(entityGuids: [\"$ENTITY_GUID\"], filter: { states: [UNRESOLVED] }, timeWindow: { startTime: $START, endTime: $NOW }, sortBy: [{ field: OCCURRENCES, direction: DESC }]${CURSOR_ARG}) { totalCount results { id name message state occurrences { totalCount expectedCount } firstSeenAt lastSeenAt eventsQuery } nextCursor } } } }"

  BODY=$(jq -n --arg q "$GQL" '{query: $q}')
  RESPONSE=$(curl -s -X POST https://api.newrelic.com/graphql \
    -H 'Content-Type: application/json' \
    -H "Api-Key: $API_KEY" \
    -d "$BODY")

  PAGE_GROUPS=$(echo "$RESPONSE" | jq '.data.actor.errorsInbox.errorGroups.results // []')
  TOTAL_COUNT=$(echo "$RESPONSE" | jq '.data.actor.errorsInbox.errorGroups.totalCount // 0')
  ALL_GROUPS=$(echo "$ALL_GROUPS $PAGE_GROUPS" | jq -s '.[0] + .[1]')
  NEXT_CURSOR=$(echo "$RESPONSE" | jq -r '.data.actor.errorsInbox.errorGroups.nextCursor // empty')

  if [ -z "$NEXT_CURSOR" ]; then
    break
  fi
  CURSOR="$NEXT_CURSOR"
done

GROUP_COUNT=$(echo "$ALL_GROUPS" | jq 'length')
echo "Found $GROUP_COUNT error groups (total: $TOTAL_COUNT)"

# Load existing JSON data (or start with empty array)
if [ -f "$JSON_FILE" ]; then
  EXISTING=$(cat "$JSON_FILE")
else
  EXISTING="[]"
fi

# --- Step 2: Fetch occurrence details and merge into JSON ---
echo "Fetching occurrence details for each error group..."

for i in $(seq 0 $(( GROUP_COUNT - 1 ))); do
  GROUP=$(echo "$ALL_GROUPS" | jq ".[$i]")
  GROUP_ID=$(echo "$GROUP" | jq -r '.id')
  NAME=$(echo "$GROUP" | jq -r '.name')
  MESSAGE=$(echo "$GROUP" | jq -r '.message')
  OCCURRENCES=$(echo "$GROUP" | jq -r '.occurrences.totalCount')
  FIRST_SEEN=$(echo "$GROUP" | jq -r '.firstSeenAt')
  LAST_SEEN=$(echo "$GROUP" | jq -r '.lastSeenAt')
  EVENTS_QUERY=$(echo "$GROUP" | jq -r '.eventsQuery')

  PERMALINK="https://one.newrelic.com/nr1-core/errors-inbox/overview/${ACCOUNT_ID}?duration=$(( SECONDS_AGO * 1000 ))&errorGroupId=${GROUP_ID}"

  # Extract WHERE conditions from eventsQuery for NRQL detail query
  WHERE_CLAUSE=$(echo "$EVENTS_QUERY" | sed 's/.*WHERE //' || true)

  echo "  [$((i + 1))/$GROUP_COUNT] $NAME ($OCCURRENCES occurrences)"

  # Fetch recent individual occurrences
  NRQL="FROM TransactionError SELECT timestamp, transactionUiName, request.uri, host, error.message, error.class, error.expected WHERE ${WHERE_CLAUSE} AND appName = 'Duunitori' SINCE ${SINCE_NRQL} ago LIMIT 5"
  NRQL_ESCAPED=$(echo "$NRQL" | sed 's/"/\\"/g')
  GQL="{ actor { account(id: $ACCOUNT_ID) { nrql(query: \"$NRQL_ESCAPED\") { results } } } }"

  BODY=$(jq -n --arg q "$GQL" '{query: $q}')
  DETAIL_RESPONSE=$(curl -s -X POST https://api.newrelic.com/graphql \
    -H 'Content-Type: application/json' \
    -H "Api-Key: $API_KEY" \
    -d "$BODY")

  DETAIL_RESULTS=$(echo "$DETAIL_RESPONSE" | jq '.data.actor.account.nrql.results // []')

  # Check if this error group already exists in our JSON
  EXISTING_INDEX=$(echo "$EXISTING" | jq --arg id "$GROUP_ID" 'to_entries | map(select(.value.id == $id)) | .[0].key // empty')

  if [ -n "$EXISTING_INDEX" ]; then
    # Update existing entry: only dates, occurrences, and recent occurrences
    EXISTING=$(echo "$EXISTING" | jq \
      --argjson idx "$EXISTING_INDEX" \
      --arg firstSeen "$FIRST_SEEN" \
      --arg lastSeen "$LAST_SEEN" \
      --argjson occurrences "$OCCURRENCES" \
      --argjson recentOccurrences "$DETAIL_RESULTS" \
      --arg link "$PERMALINK" \
      '.[$idx].firstSeen = $firstSeen | .[$idx].lastSeen = $lastSeen | .[$idx].occurrences = $occurrences | .[$idx].recentOccurrences = $recentOccurrences | .[$idx].link = $link')
  else
    # Append new entry with status: todo
    NEW_ENTRY=$(jq -n \
      --arg id "$GROUP_ID" \
      --arg name "$NAME" \
      --arg message "$MESSAGE" \
      --arg firstSeen "$FIRST_SEEN" \
      --arg lastSeen "$LAST_SEEN" \
      --argjson occurrences "$OCCURRENCES" \
      --arg eventsQuery "$EVENTS_QUERY" \
      --arg link "$PERMALINK" \
      --argjson recentOccurrences "$DETAIL_RESULTS" \
      '{id: $id, name: $name, message: $message, status: "todo", occurrences: $occurrences, firstSeen: $firstSeen, lastSeen: $lastSeen, eventsQuery: $eventsQuery, link: $link, recentOccurrences: $recentOccurrences}')
    EXISTING=$(echo "$EXISTING" | jq --argjson entry "$NEW_ENTRY" '. + [$entry]')
  fi
done

# Save merged JSON
echo "$EXISTING" | jq '.' > "$JSON_FILE"

ENTRY_COUNT=$(echo "$EXISTING" | jq 'length')
echo ""
echo "Done. $ENTRY_COUNT errors saved to $JSON_FILE"
