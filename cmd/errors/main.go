package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/petrihanninen/errors/internal/agent"
	"github.com/petrihanninen/errors/internal/config"
	"github.com/petrihanninen/errors/internal/db"
	"github.com/petrihanninen/errors/internal/gitops"
	"github.com/petrihanninen/errors/internal/newrelic"
	"github.com/petrihanninen/errors/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <fetch|fix|serve>\n", os.Args[0])
		os.Exit(1)
	}

	switch os.Args[1] {
	case "fetch":
		if err := runFetch(); err != nil {
			log.Fatalf("fetch failed: %v", err)
		}
	case "fix":
		if err := runFix(); err != nil {
			log.Fatalf("fix failed: %v", err)
		}
	case "serve":
		if err := runServe(); err != nil {
			log.Fatalf("serve failed: %v", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nUsage: %s <fetch|fix|serve>\n", os.Args[1], os.Args[0])
		os.Exit(1)
	}
}

func runFetch() error {
	cfg := config.Load()
	if err := cfg.ValidateFetch(); err != nil {
		return err
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	store := db.NewStore(pool)
	nrClient := newrelic.NewClient(cfg.NewRelicAccountID, cfg.NewRelicAPIKey, cfg.NewRelicEntityGUID)

	since := 24 * time.Hour
	log.Println("Fetching unresolved errors from the last 24 hours...")

	groups, totalCount, err := nrClient.FetchErrorGroups(ctx, since)
	if err != nil {
		return fmt.Errorf("fetch error groups: %w", err)
	}
	log.Printf("Found %d error groups (total: %d)", len(groups), totalCount)

	for i, g := range groups {
		log.Printf("  [%d/%d] %s (%d occurrences)", i+1, len(groups), g.Name, g.Occurrences.TotalCount)

		permalink := nrClient.BuildPermalink(g.ID, since)

		eg := &db.ErrorGroup{
			ID:          g.ID,
			Name:        g.Name,
			Message:     g.Message,
			Status:      "todo",
			Occurrences: g.Occurrences.TotalCount,
			FirstSeen:   g.FirstSeenAt,
			LastSeen:    g.LastSeenAt,
			EventsQuery: g.EventsQuery,
			Link:        permalink,
		}

		if err := store.UpsertErrorGroup(ctx, eg); err != nil {
			log.Printf("    Warning: failed to upsert error group %s: %v", g.ID, err)
			continue
		}

		occs, err := nrClient.FetchOccurrenceDetails(ctx, g.EventsQuery, since)
		if err != nil {
			log.Printf("    Warning: failed to fetch occurrences for %s: %v", g.Name, err)
			continue
		}

		for _, o := range occs {
			eo := &db.ErrorOccurrence{
				ErrorGroupID:    g.ID,
				ErrorClass:      o.ErrorClass,
				Message:         o.ErrorMessage,
				Host:            o.Host,
				RequestURI:      o.RequestURI,
				TransactionName: o.TransactionName,
				OccurredAt:      int64(o.Timestamp),
			}
			if err := store.UpsertErrorOccurrence(ctx, eo); err != nil {
				log.Printf("    Warning: failed to upsert occurrence: %v", err)
			}
		}
	}

	log.Println("Fetch complete.")
	return nil
}

func runFix() error {
	cfg := config.Load()
	if err := cfg.ValidateFix(); err != nil {
		return err
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	store := db.NewStore(pool)

	// Load custom prompt from DB, fall back to default
	customPrompt, err := store.GetSetting(ctx, "system_prompt")
	if err != nil {
		customPrompt = ""
	}

	todoErrors, err := store.ListTodoErrorGroups(ctx)
	if err != nil {
		return fmt.Errorf("list todo errors: %w", err)
	}

	log.Printf("Found %d todo error(s)", len(todoErrors))
	if len(todoErrors) == 0 {
		log.Println("No todo errors. Done!")
		return nil
	}

	workDir := "/tmp/errors-workdir"
	os.RemoveAll(workDir)
	repoDir := filepath.Join(workDir, "duunitori5")

	log.Printf("Cloning %s to %s...", cfg.DuunitoriRepo, repoDir)
	repo, err := gitops.Clone(cfg.DuunitoriRepo, cfg.GithubToken, cfg.DuunitoriBaseBranch, repoDir)
	if err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}

	fixAgent := agent.New(cfg.AnthropicAPIKey, repoDir, customPrompt)

	var doneCount, failedCount int

	for i, eg := range todoErrors {
		log.Printf("\n============================================")
		log.Printf("Processing error %d/%d", i+1, len(todoErrors))
		log.Printf("  Name:    %s", eg.Name)
		log.Printf("  Message: %s", eg.Message)
		log.Printf("============================================")

		if err := store.SetErrorGroupStatus(ctx, eg.ID, "doing"); err != nil {
			log.Printf("Warning: failed to set status to doing: %v", err)
		}

		branchName := gitops.BranchNameFromError(eg.Name, eg.Message)

		attemptID, err := store.CreateFixAttempt(ctx, eg.ID, branchName)
		if err != nil {
			log.Printf("Warning: failed to create fix attempt: %v", err)
		}

		if err := repo.CreateBranch(branchName); err != nil {
			log.Printf("Error creating branch: %v", err)
			markFailed(ctx, store, eg.ID, attemptID, "failed to create branch")
			failedCount++
			continue
		}
		log.Printf("Created branch: %s", branchName)

		occs, err := store.GetOccurrencesForGroup(ctx, eg.ID)
		if err != nil {
			log.Printf("Warning: failed to get occurrences: %v", err)
		}

		log.Println("Running agent to fix the error...")
		result, err := fixAgent.Fix(ctx, &eg, occs)
		if err != nil {
			log.Printf("Agent error: %v", err)
			repo.Cleanup(branchName)
			markFailed(ctx, store, eg.ID, attemptID, fmt.Sprintf("agent error: %v", err))
			failedCount++
			continue
		}

		if result.CannotFix {
			log.Println("Agent could not fix this error. Marking as failed.")
			repo.Cleanup(branchName)
			markFailed(ctx, store, eg.ID, attemptID, result.Output)
			failedCount++
			continue
		}

		hasChanges, err := repo.HasChanges()
		if err != nil {
			log.Printf("Error checking changes: %v", err)
			repo.Cleanup(branchName)
			markFailed(ctx, store, eg.ID, attemptID, result.Output)
			failedCount++
			continue
		}

		if !hasChanges {
			log.Println("No changes were made. Marking as failed.")
			repo.Cleanup(branchName)
			markFailed(ctx, store, eg.ID, attemptID, result.Output)
			failedCount++
			continue
		}

		commitMsg := fmt.Sprintf("Fix production error: %s\n\n%s\n\nCo-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>", eg.Name, eg.Message)
		sha, err := repo.CommitAll(commitMsg)
		if err != nil {
			log.Printf("Error committing: %v", err)
			repo.Cleanup(branchName)
			markFailed(ctx, store, eg.ID, attemptID, result.Output)
			failedCount++
			continue
		}

		if err := repo.Push(branchName); err != nil {
			log.Printf("Error pushing: %v", err)
			repo.Cleanup(branchName)
			markFailed(ctx, store, eg.ID, attemptID, result.Output)
			failedCount++
			continue
		}

		log.Printf("Changes committed and pushed on branch: %s (SHA: %s)", branchName, sha)

		if err := store.SetErrorGroupStatus(ctx, eg.ID, "done"); err != nil {
			log.Printf("Warning: failed to set status to done: %v", err)
		}
		if attemptID > 0 {
			store.CompleteFixAttempt(ctx, attemptID, "success", result.Output, sha)
		}
		doneCount++

		repo.Cleanup(branchName)
	}

	log.Printf("\n============================================")
	log.Printf("Summary:")
	log.Printf("  Done:   %d", doneCount)
	log.Printf("  Failed: %d", failedCount)
	log.Printf("  Todo:   %d", len(todoErrors)-doneCount-failedCount)
	log.Printf("============================================")

	return nil
}

func runServe() error {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	store := db.NewStore(pool)

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	log.Printf("Starting web server on %s", addr)
	return server.ListenAndServe(addr, store)
}

func markFailed(ctx context.Context, store *db.Store, errorGroupID string, attemptID int, output string) {
	store.SetErrorGroupStatus(ctx, errorGroupID, "failed")
	if attemptID > 0 {
		store.CompleteFixAttempt(ctx, attemptID, "failed", output, "")
	}
}
