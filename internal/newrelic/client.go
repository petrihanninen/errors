package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	accountID  string
	apiKey     string
	entityGUID string
	httpClient *http.Client
}

func NewClient(accountID, apiKey, entityGUID string) *Client {
	return &Client{
		accountID:  accountID,
		apiKey:     apiKey,
		entityGUID: entityGUID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) FetchErrorGroups(ctx context.Context, since time.Duration) ([]ErrorGroup, int, error) {
	now := time.Now().UnixMilli()
	start := time.Now().Add(-since).UnixMilli()

	var allGroups []ErrorGroup
	var cursor *string
	var totalCount int

	for {
		cursorArg := ""
		if cursor != nil {
			cursorArg = fmt.Sprintf(`, cursor: "%s"`, *cursor)
		}

		query := fmt.Sprintf(`{
			actor {
				errorsInbox {
					errorGroups(
						entityGuids: ["%s"],
						filter: { states: [UNRESOLVED] },
						timeWindow: { startTime: %d, endTime: %d },
						sortBy: [{ field: OCCURRENCES, direction: DESC }]
						%s
					) {
						totalCount
						results {
							id name message state
							occurrences { totalCount expectedCount }
							firstSeenAt lastSeenAt eventsQuery
						}
						nextCursor
					}
				}
			}
		}`, c.entityGUID, start, now, cursorArg)

		resp, err := c.graphqlRequest(ctx, query)
		if err != nil {
			return nil, 0, fmt.Errorf("fetch error groups: %w", err)
		}

		groups := resp.Data.Actor.ErrorsInbox.ErrorGroups
		totalCount = groups.TotalCount
		allGroups = append(allGroups, groups.Results...)

		if groups.NextCursor == nil || *groups.NextCursor == "" {
			break
		}
		cursor = groups.NextCursor
	}

	return allGroups, totalCount, nil
}

func (c *Client) FetchOccurrenceDetails(ctx context.Context, eventsQuery string, since time.Duration) ([]OccurrenceDetail, error) {
	whereClause := extractWhereClause(eventsQuery)
	sinceStr := formatDuration(since)

	nrql := fmt.Sprintf(
		"FROM TransactionError SELECT timestamp, transactionUiName, request.uri, host, error.message, error.class, error.expected WHERE %s AND appName = 'Duunitori' SINCE %s ago LIMIT 5",
		whereClause, sinceStr,
	)

	escaped := strings.ReplaceAll(nrql, `"`, `\"`)
	query := fmt.Sprintf(`{
		actor {
			account(id: %s) {
				nrql(query: "%s") {
					results
				}
			}
		}
	}`, c.accountID, escaped)

	resp, err := c.graphqlRequest(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("fetch occurrence details: %w", err)
	}

	results := resp.Data.Actor.Account.NRQL.Results
	details := make([]OccurrenceDetail, 0, len(results))

	for _, r := range results {
		d := OccurrenceDetail{}
		if v, ok := r["error.class"].(string); ok {
			d.ErrorClass = v
		}
		if v, ok := r["error.expected"].(bool); ok {
			d.ErrorExpected = v
		}
		if v, ok := r["error.message"].(string); ok {
			d.ErrorMessage = v
		}
		if v, ok := r["host"].(string); ok {
			d.Host = v
		}
		if v, ok := r["request.uri"].(string); ok {
			d.RequestURI = v
		}
		if v, ok := r["timestamp"].(float64); ok {
			d.Timestamp = v
		}
		if v, ok := r["transactionUiName"].(string); ok {
			d.TransactionName = v
		}
		details = append(details, d)
	}

	return details, nil
}

func (c *Client) BuildPermalink(groupID string, since time.Duration) string {
	durationMs := int64(since.Seconds()) * 1000
	return fmt.Sprintf("https://one.newrelic.com/nr1-core/errors-inbox/overview/%s?duration=%d&errorGroupId=%s",
		c.accountID, durationMs, groupID)
}

func (c *Client) graphqlRequest(ctx context.Context, query string) (*GraphQLResponse, error) {
	body := map[string]string{"query": query}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.newrelic.com/graphql", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result GraphQLResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

func extractWhereClause(eventsQuery string) string {
	idx := strings.Index(strings.ToUpper(eventsQuery), "WHERE ")
	if idx >= 0 {
		return eventsQuery[idx+6:]
	}
	return eventsQuery
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	if hours >= 24 && hours%24 == 0 {
		return fmt.Sprintf("%d days", hours/24)
	}
	return fmt.Sprintf("%d hours", hours)
}
