package gitops

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type Repo struct {
	dir        string
	baseBranch string
}

func Clone(repoURL, token, baseBranch, destDir string) (*Repo, error) {
	// Insert token into HTTPS URL for auth
	authURL := repoURL
	if token != "" && strings.HasPrefix(repoURL, "https://") {
		authURL = strings.Replace(repoURL, "https://", fmt.Sprintf("https://%s@", token), 1)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("create dest dir: %w", err)
	}

	// Shallow clone
	if err := run("git", "clone", "--depth=1", "--branch", baseBranch, authURL, destDir); err != nil {
		return nil, fmt.Errorf("clone repo: %w", err)
	}

	return &Repo{dir: destDir, baseBranch: baseBranch}, nil
}

func (r *Repo) CreateBranch(name string) error {
	if err := r.runInDir("git", "checkout", "-b", name); err != nil {
		return fmt.Errorf("create branch %s: %w", name, err)
	}
	return nil
}

func (r *Repo) HasChanges() (bool, error) {
	cmd := exec.Command("git", "diff", "--quiet")
	cmd.Dir = r.dir
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true, nil // has unstaged changes
		}
		return false, err
	}

	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = r.dir
	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true, nil // has staged changes
		}
		return false, err
	}

	// Also check for untracked files
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.dir
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

func (r *Repo) CommitAll(message string) (string, error) {
	if err := r.runInDir("git", "add", "-A"); err != nil {
		return "", fmt.Errorf("stage changes: %w", err)
	}
	if err := r.runInDir("git", "commit", "-m", message); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	// Get commit SHA
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = r.dir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get commit SHA: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (r *Repo) Push(branchName string) error {
	if err := r.runInDir("git", "push", "origin", branchName); err != nil {
		return fmt.Errorf("push branch %s: %w", branchName, err)
	}
	return nil
}

func (r *Repo) Cleanup(branchName string) error {
	_ = r.runInDir("git", "checkout", r.baseBranch)
	_ = r.runInDir("git", "branch", "-D", branchName)
	return nil
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)
var multiDash = regexp.MustCompile(`-{2,}`)

func BranchNameFromError(name, message string) string {
	raw := name + "--" + message
	raw = strings.ToLower(raw)
	raw = nonAlphanumeric.ReplaceAllString(raw, "-")
	raw = multiDash.ReplaceAllString(raw, "-")
	raw = strings.Trim(raw, "-")
	if len(raw) > 60 {
		raw = raw[:60]
		raw = strings.TrimRight(raw, "-")
	}
	return "auto-fix/" + raw
}

func (r *Repo) runInDir(name string, args ...string) error {
	return runDir(r.dir, name, args...)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
