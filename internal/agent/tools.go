package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

func toolDefinitions() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{OfTool: &anthropic.ToolParam{
			Name:        "read_file",
			Description: anthropic.String("Read the contents of a file. Returns the full file content."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: marshalProps(map[string]propDef{
					"path": {Type: "string", Description: "The file path relative to the repository root"},
				}),
			},
		}},
		{OfTool: &anthropic.ToolParam{
			Name:        "write_file",
			Description: anthropic.String("Write content to a file. Creates the file if it doesn't exist, overwrites if it does."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: marshalProps(map[string]propDef{
					"path":    {Type: "string", Description: "The file path relative to the repository root"},
					"content": {Type: "string", Description: "The content to write to the file"},
				}),
			},
		}},
		{OfTool: &anthropic.ToolParam{
			Name:        "list_directory",
			Description: anthropic.String("List files and directories in the given path."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: marshalProps(map[string]propDef{
					"path": {Type: "string", Description: "The directory path relative to the repository root. Use '.' for root."},
				}),
			},
		}},
		{OfTool: &anthropic.ToolParam{
			Name:        "search_files",
			Description: anthropic.String("Search for a pattern in files using grep. Returns matching lines with file paths and line numbers."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: marshalProps(map[string]propDef{
					"pattern":   {Type: "string", Description: "The search pattern (grep regex)"},
					"path":      {Type: "string", Description: "Directory to search in, relative to repo root. Use '.' for entire repo."},
					"file_glob": {Type: "string", Description: "Optional glob pattern for file names (e.g. '*.py')"},
				}),
			},
		}},
		{OfTool: &anthropic.ToolParam{
			Name:        "run_command",
			Description: anthropic.String("Run a shell command in the repository root directory. Use for running tests, linters, or other development commands."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: marshalProps(map[string]propDef{
					"command": {Type: "string", Description: "The shell command to run"},
				}),
			},
		}},
	}
}

type propDef struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

func marshalProps(props map[string]propDef) interface{} {
	m := make(map[string]interface{})
	for k, v := range props {
		m[k] = map[string]string{
			"type":        v.Type,
			"description": v.Description,
		}
	}
	return m
}

func executeTool(name string, input json.RawMessage, repoDir string) (string, error) {
	var params map[string]string
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse tool input: %w", err)
	}

	switch name {
	case "read_file":
		return execReadFile(params["path"], repoDir)
	case "write_file":
		return execWriteFile(params["path"], params["content"], repoDir)
	case "list_directory":
		return execListDirectory(params["path"], repoDir)
	case "search_files":
		return execSearchFiles(params["pattern"], params["path"], params["file_glob"], repoDir)
	case "run_command":
		return execRunCommand(params["command"], repoDir)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func resolvePath(path, repoDir string) (string, error) {
	if path == "" {
		path = "."
	}
	resolved := filepath.Join(repoDir, path)
	resolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	// Ensure path stays within repo directory
	if !strings.HasPrefix(resolved, repoDir) {
		return "", fmt.Errorf("path %q escapes repository directory", path)
	}
	return resolved, nil
}

func execReadFile(path, repoDir string) (string, error) {
	resolved, err := resolvePath(path, repoDir)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

func execWriteFile(path, content, repoDir string) (string, error) {
	resolved, err := resolvePath(path, repoDir)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(resolved)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directories: %w", err)
	}
	if err := os.WriteFile(resolved, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("File written: %s", path), nil
}

func execListDirectory(path, repoDir string) (string, error) {
	resolved, err := resolvePath(path, repoDir)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return "", fmt.Errorf("list directory: %w", err)
	}
	var lines []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	return strings.Join(lines, "\n"), nil
}

func execSearchFiles(pattern, path, fileGlob, repoDir string) (string, error) {
	resolved, err := resolvePath(path, repoDir)
	if err != nil {
		return "", err
	}

	args := []string{"-rn", "--color=never"}
	if fileGlob != "" {
		args = append(args, "--include="+fileGlob)
	}
	args = append(args, pattern, resolved)

	cmd := exec.Command("grep", args...)
	output, err := cmd.CombinedOutput()

	// grep returns exit code 1 when no matches found
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "No matches found.", nil
		}
		return "", fmt.Errorf("search failed: %w\n%s", err, string(output))
	}

	result := string(output)
	// Replace absolute paths with relative paths for cleaner output
	result = strings.ReplaceAll(result, repoDir+"/", "")

	// Truncate if too long
	if len(result) > 50000 {
		result = result[:50000] + "\n... (output truncated)"
	}

	return result, nil
}

func execRunCommand(command, repoDir string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	result := string(output)

	if len(result) > 50000 {
		result = result[:50000] + "\n... (output truncated)"
	}

	if err != nil {
		return fmt.Sprintf("%s\nCommand exited with error: %v", result, err), nil
	}
	return result, nil
}
