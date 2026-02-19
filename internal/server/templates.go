package server

const pageHead = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Errors Dashboard</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #0d1117; color: #c9d1d9; line-height: 1.5; }
  a { color: #58a6ff; text-decoration: none; }
  a:hover { text-decoration: underline; }
  .container { max-width: 1000px; margin: 0 auto; padding: 20px; }
  nav { background: #161b22; border-bottom: 1px solid #30363d; padding: 12px 0; margin-bottom: 24px; }
  nav .container { display: flex; gap: 24px; align-items: center; }
  nav a { color: #c9d1d9; font-weight: 500; }
  nav a:hover { color: #fff; text-decoration: none; }
  nav .brand { font-weight: 700; font-size: 1.1em; color: #fff; }
  h1 { font-size: 1.4em; margin-bottom: 16px; color: #fff; }
  h2 { font-size: 1.1em; margin: 24px 0 12px; color: #fff; }
  table { width: 100%; border-collapse: collapse; background: #161b22; border: 1px solid #30363d; border-radius: 6px; overflow: hidden; margin-bottom: 24px; }
  th { text-align: left; padding: 10px 12px; background: #21262d; border-bottom: 1px solid #30363d; font-size: 0.85em; color: #8b949e; font-weight: 600; }
  td { padding: 10px 12px; border-bottom: 1px solid #21262d; font-size: 0.9em; }
  tr:last-child td { border-bottom: none; }
  tr:hover td { background: #1c2129; }
  .badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 0.8em; font-weight: 600; }
  .status-done { background: #238636; color: #fff; }
  .status-failed { background: #da3633; color: #fff; }
  .status-doing { background: #d29922; color: #000; }
  .status-todo { background: #388bfd; color: #fff; }
  .mono { font-family: "SF Mono", "Fira Code", monospace; font-size: 0.85em; }
  .text-muted { color: #8b949e; }
  textarea { width: 100%; min-height: 300px; background: #0d1117; color: #c9d1d9; border: 1px solid #30363d; border-radius: 6px; padding: 12px; font-family: "SF Mono", "Fira Code", monospace; font-size: 0.9em; resize: vertical; }
  textarea:focus { outline: none; border-color: #58a6ff; }
  button { background: #238636; color: #fff; border: none; padding: 8px 20px; border-radius: 6px; font-size: 0.9em; cursor: pointer; font-weight: 600; }
  button:hover { background: #2ea043; }
  .btn-secondary { background: #30363d; }
  .btn-secondary:hover { background: #3d444d; }
  .alert { padding: 12px 16px; border-radius: 6px; margin-bottom: 16px; background: #1b3826; border: 1px solid #238636; color: #3fb950; }
  .card { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 16px; margin-bottom: 16px; }
  .card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
  pre { background: #0d1117; border: 1px solid #30363d; border-radius: 6px; padding: 12px; overflow-x: auto; font-size: 0.85em; max-height: 400px; overflow-y: auto; }
  dl { display: grid; grid-template-columns: auto 1fr; gap: 4px 16px; }
  dt { color: #8b949e; font-size: 0.85em; }
  dd { font-size: 0.9em; }
</style>
</head>
<body>
<nav>
  <div class="container">
    <span class="brand">Errors</span>
    <a href="/">Dashboard</a>
    <a href="/branches">Branches</a>
    <a href="/prompt">Prompt</a>
  </div>
</nav>
<div class="container">`

const pageFoot = `</div>
</body>
</html>`

const indexTemplate = pageHead + `
<h1>Error Groups</h1>
<table>
  <thead>
    <tr>
      <th>Status</th>
      <th>Error</th>
      <th>Message</th>
      <th>Count</th>
      <th>Last Seen</th>
    </tr>
  </thead>
  <tbody>
    {{range .Groups}}
    <tr>
      <td><span class="badge {{statusClass .Status}}">{{.Status}}</span></td>
      <td><a href="/error/{{.ID}}">{{.Name}}</a></td>
      <td class="text-muted" style="max-width:300px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{{.Message}}</td>
      <td>{{.Occurrences}}</td>
      <td class="text-muted">{{formatTimestamp .LastSeen}}</td>
    </tr>
    {{else}}
    <tr><td colspan="5" class="text-muted" style="text-align:center;">No errors found. Run <code>errors fetch</code> to populate.</td></tr>
    {{end}}
  </tbody>
</table>
` + pageFoot

const errorDetailTemplate = pageHead + `
<h1>{{.Group.Name}}</h1>
<div class="card">
  <dl>
    <dt>Status</dt><dd><span class="badge {{statusClass .Group.Status}}">{{.Group.Status}}</span></dd>
    <dt>Message</dt><dd>{{.Group.Message}}</dd>
    <dt>Occurrences</dt><dd>{{.Group.Occurrences}}</dd>
    <dt>First Seen</dt><dd>{{formatTimestamp .Group.FirstSeen}}</dd>
    <dt>Last Seen</dt><dd>{{formatTimestamp .Group.LastSeen}}</dd>
    <dt>New Relic</dt><dd><a href="{{.Group.Link}}" target="_blank">View in New Relic</a></dd>
  </dl>
</div>

<h2>Recent Occurrences</h2>
{{if .Occurrences}}
<table>
  <thead>
    <tr>
      <th>Class</th>
      <th>Message</th>
      <th>Request URI</th>
      <th>Transaction</th>
      <th>Host</th>
      <th>Time</th>
    </tr>
  </thead>
  <tbody>
    {{range .Occurrences}}
    <tr>
      <td class="mono">{{.ErrorClass}}</td>
      <td style="max-width:200px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{{.Message}}</td>
      <td class="mono">{{.RequestURI}}</td>
      <td class="mono">{{.TransactionName}}</td>
      <td class="mono text-muted">{{.Host}}</td>
      <td class="text-muted">{{formatTimestamp .OccurredAt}}</td>
    </tr>
    {{end}}
  </tbody>
</table>
{{else}}
<p class="text-muted">No occurrences recorded.</p>
{{end}}

<h2>Fix Attempts</h2>
{{if .Attempts}}
<table>
  <thead>
    <tr>
      <th>Status</th>
      <th>Branch</th>
      <th>Commit</th>
      <th>Started</th>
      <th>Completed</th>
    </tr>
  </thead>
  <tbody>
    {{range .Attempts}}
    <tr>
      <td><span class="badge {{statusClass .Status}}">{{.Status}}</span></td>
      <td class="mono">{{.BranchName}}</td>
      <td class="mono">{{if .CommitSHA}}{{truncateOutput .CommitSHA 8}}{{else}}-{{end}}</td>
      <td class="text-muted">{{formatTime .StartedAt}}</td>
      <td class="text-muted">{{formatTimePtr .CompletedAt}}</td>
    </tr>
    {{end}}
  </tbody>
</table>
{{else}}
<p class="text-muted">No fix attempts yet.</p>
{{end}}
` + pageFoot

const branchesTemplate = pageHead + `
<h1>Branches</h1>
<table>
  <thead>
    <tr>
      <th>Status</th>
      <th>Branch</th>
      <th>Error</th>
      <th>Commit</th>
      <th>Started</th>
      <th>Completed</th>
    </tr>
  </thead>
  <tbody>
    {{range .Branches}}
    <tr>
      <td><span class="badge {{statusClass .Status}}">{{.Status}}</span></td>
      <td class="mono">{{.BranchName}}</td>
      <td><a href="/error/{{.ErrorGroupID}}">{{.ErrorName}}</a></td>
      <td class="mono">{{if .CommitSHA}}{{truncateOutput .CommitSHA 8}}{{else}}-{{end}}</td>
      <td class="text-muted">{{formatTime .StartedAt}}</td>
      <td class="text-muted">{{formatTimePtr .CompletedAt}}</td>
    </tr>
    {{else}}
    <tr><td colspan="6" class="text-muted" style="text-align:center;">No fix attempts yet.</td></tr>
    {{end}}
  </tbody>
</table>
` + pageFoot

const promptTemplate = pageHead + `
<h1>Agent System Prompt</h1>
{{if .Saved}}
<div class="alert">Prompt saved successfully.</div>
{{end}}
<form method="POST" action="/prompt">
  <p class="text-muted" style="margin-bottom: 12px;">This prompt is sent as the system message to Claude when fixing errors. Changes take effect on the next <code>errors fix</code> run.</p>
  <textarea name="prompt">{{.Prompt}}</textarea>
  <div style="margin-top: 12px; display: flex; gap: 8px;">
    <button type="submit">Save Prompt</button>
    <button type="button" class="btn-secondary" onclick="document.querySelector('textarea').value = defaultPrompt">Reset to Default</button>
  </div>
</form>
<script>
const defaultPrompt = ` + "`" + `{{.Default}}` + "`" + `;
</script>
` + pageFoot
