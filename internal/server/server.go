package server

import (
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/petrihanninen/errors/internal/agent"
	"github.com/petrihanninen/errors/internal/db"
)

func ListenAndServe(addr string, store *db.Store) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex(store))
	mux.HandleFunc("/error/", handleErrorDetail(store))
	mux.HandleFunc("/prompt", handlePrompt(store))
	mux.HandleFunc("/branches", handleBranches(store))
	return http.ListenAndServe(addr, mux)
}

func handleIndex(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		ctx := r.Context()
		groups, err := store.ListAllErrorGroups(ctx)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		data := struct {
			Groups []db.ErrorGroup
		}{Groups: groups}
		renderTemplate(w, indexTemplate, data)
	}
}

func handleErrorDetail(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/error/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		ctx := r.Context()

		groups, err := store.ListAllErrorGroups(ctx)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var group *db.ErrorGroup
		for _, g := range groups {
			if g.ID == id {
				group = &g
				break
			}
		}
		if group == nil {
			http.NotFound(w, r)
			return
		}

		occs, err := store.GetOccurrencesForGroup(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		attempts, err := store.ListFixAttemptsForGroup(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		data := struct {
			Group       db.ErrorGroup
			Occurrences []db.ErrorOccurrence
			Attempts    []db.FixAttempt
		}{
			Group:       *group,
			Occurrences: occs,
			Attempts:    attempts,
		}
		renderTemplate(w, errorDetailTemplate, data)
	}
}

func handlePrompt(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if r.Method == "POST" {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			prompt := r.FormValue("prompt")
			if err := store.SetSetting(ctx, "system_prompt", prompt); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			http.Redirect(w, r, "/prompt?saved=1", http.StatusSeeOther)
			return
		}

		currentPrompt, err := store.GetSetting(ctx, "system_prompt")
		if err != nil {
			currentPrompt = agent.DefaultSystemPrompt
		}

		data := struct {
			Prompt  string
			Default string
			Saved   bool
		}{
			Prompt:  currentPrompt,
			Default: agent.DefaultSystemPrompt,
			Saved:   r.URL.Query().Get("saved") == "1",
		}
		renderTemplate(w, promptTemplate, data)
	}
}

func handleBranches(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		attempts, err := store.ListAllFixAttempts(ctx)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Build a map of error group ID -> name for display
		groups, _ := store.ListAllErrorGroups(ctx)
		groupNames := make(map[string]string)
		for _, g := range groups {
			groupNames[g.ID] = g.Name
		}

		type branchRow struct {
			db.FixAttempt
			ErrorName string
		}
		rows := make([]branchRow, len(attempts))
		for i, a := range attempts {
			rows[i] = branchRow{
				FixAttempt: a,
				ErrorName:  groupNames[a.ErrorGroupID],
			}
		}

		data := struct {
			Branches []branchRow
		}{Branches: rows}
		renderTemplate(w, branchesTemplate, data)
	}
}

func renderTemplate(w http.ResponseWriter, tmplStr string, data interface{}) {
	funcMap := template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04")
		},
		"formatTimePtr": func(t *time.Time) string {
			if t == nil {
				return "-"
			}
			return t.Format("2006-01-02 15:04")
		},
		"formatTimestamp": func(ts int64) string {
			if ts == 0 {
				return "-"
			}
			return time.UnixMilli(ts).Format("2006-01-02 15:04")
		},
		"statusClass": func(s string) string {
			switch s {
			case "done", "success":
				return "status-done"
			case "failed":
				return "status-failed"
			case "doing", "running":
				return "status-doing"
			default:
				return "status-todo"
			}
		},
		"truncateOutput": func(s string, n int) string {
			if len(s) > n {
				return s[:n] + "..."
			}
			return s
		},
	}

	tmpl, err := template.New("page").Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		log.Printf("Template parse error: %v", err)
		http.Error(w, "internal server error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execute error: %v", err)
	}
}
