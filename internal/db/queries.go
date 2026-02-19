package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ErrorGroup struct {
	ID          string
	Name        string
	Message     string
	Status      string
	Occurrences int
	FirstSeen   int64
	LastSeen    int64
	EventsQuery string
	Link        string
}

type ErrorOccurrence struct {
	ErrorGroupID    string
	ErrorClass      string
	Message         string
	Host            string
	RequestURI      string
	TransactionName string
	OccurredAt      int64
}

type FixAttempt struct {
	ID           int
	ErrorGroupID string
	BranchName   string
	Status       string
	AgentOutput  string
	CommitSHA    string
	StartedAt    time.Time
	CompletedAt  *time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) UpsertErrorGroup(ctx context.Context, eg *ErrorGroup) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO error_groups (id, name, message, status, occurrences, first_seen, last_seen, events_query, link)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			occurrences = EXCLUDED.occurrences,
			first_seen = EXCLUDED.first_seen,
			last_seen = EXCLUDED.last_seen,
			link = EXCLUDED.link,
			updated_at = NOW()
	`, eg.ID, eg.Name, eg.Message, eg.Status, eg.Occurrences, eg.FirstSeen, eg.LastSeen, eg.EventsQuery, eg.Link)
	return err
}

func (s *Store) UpsertErrorOccurrence(ctx context.Context, eo *ErrorOccurrence) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO error_occurrences (error_group_id, error_class, message, host, request_uri, transaction_name, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (error_group_id, occurred_at, host) DO NOTHING
	`, eo.ErrorGroupID, eo.ErrorClass, eo.Message, eo.Host, eo.RequestURI, eo.TransactionName, eo.OccurredAt)
	return err
}

func (s *Store) ListTodoErrorGroups(ctx context.Context) ([]ErrorGroup, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, message, status, occurrences, first_seen, last_seen, events_query, link
		FROM error_groups
		WHERE status = 'todo'
		ORDER BY occurrences DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []ErrorGroup
	for rows.Next() {
		var g ErrorGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Message, &g.Status, &g.Occurrences, &g.FirstSeen, &g.LastSeen, &g.EventsQuery, &g.Link); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *Store) SetErrorGroupStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE error_groups SET status = $2, updated_at = NOW() WHERE id = $1
	`, id, status)
	return err
}

func (s *Store) CreateFixAttempt(ctx context.Context, errorGroupID, branchName string) (int, error) {
	var id int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO fix_attempts (error_group_id, branch_name)
		VALUES ($1, $2)
		RETURNING id
	`, errorGroupID, branchName).Scan(&id)
	return id, err
}

func (s *Store) CompleteFixAttempt(ctx context.Context, id int, status, agentOutput, commitSHA string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE fix_attempts
		SET status = $2, agent_output = $3, commit_sha = $4, completed_at = NOW()
		WHERE id = $1
	`, id, status, agentOutput, commitSHA)
	return err
}

func (s *Store) GetOccurrencesForGroup(ctx context.Context, errorGroupID string) ([]ErrorOccurrence, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT error_group_id, error_class, message, host, request_uri, transaction_name, occurred_at
		FROM error_occurrences
		WHERE error_group_id = $1
		ORDER BY occurred_at DESC
		LIMIT 10
	`, errorGroupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var occs []ErrorOccurrence
	for rows.Next() {
		var o ErrorOccurrence
		if err := rows.Scan(&o.ErrorGroupID, &o.ErrorClass, &o.Message, &o.Host, &o.RequestURI, &o.TransactionName, &o.OccurredAt); err != nil {
			return nil, err
		}
		occs = append(occs, o)
	}
	return occs, rows.Err()
}

func (s *Store) ListAllErrorGroups(ctx context.Context) ([]ErrorGroup, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, message, status, occurrences, first_seen, last_seen, events_query, link
		FROM error_groups
		ORDER BY last_seen DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []ErrorGroup
	for rows.Next() {
		var g ErrorGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Message, &g.Status, &g.Occurrences, &g.FirstSeen, &g.LastSeen, &g.EventsQuery, &g.Link); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *Store) ListFixAttemptsForGroup(ctx context.Context, errorGroupID string) ([]FixAttempt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, error_group_id, branch_name, status, COALESCE(agent_output, ''), COALESCE(commit_sha, ''), started_at, completed_at
		FROM fix_attempts
		WHERE error_group_id = $1
		ORDER BY started_at DESC
	`, errorGroupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []FixAttempt
	for rows.Next() {
		var a FixAttempt
		if err := rows.Scan(&a.ID, &a.ErrorGroupID, &a.BranchName, &a.Status, &a.AgentOutput, &a.CommitSHA, &a.StartedAt, &a.CompletedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, key, value)
	return err
}

func (s *Store) ListAllFixAttempts(ctx context.Context) ([]FixAttempt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, error_group_id, branch_name, status, COALESCE(agent_output, ''), COALESCE(commit_sha, ''), started_at, completed_at
		FROM fix_attempts
		ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []FixAttempt
	for rows.Next() {
		var a FixAttempt
		if err := rows.Scan(&a.ID, &a.ErrorGroupID, &a.BranchName, &a.Status, &a.AgentOutput, &a.CommitSHA, &a.StartedAt, &a.CompletedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}
