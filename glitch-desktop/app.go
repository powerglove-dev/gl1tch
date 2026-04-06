package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/8op-org/gl1tch/pkg/glitchd"
)

type App struct {
	ctx           context.Context
	cancelBackend context.CancelFunc
	notifyProc    *os.Process
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	bgCtx, cancel := context.WithCancel(context.Background())
	a.cancelBackend = cancel

	go func() {
		if err := glitchd.RunBackend(bgCtx); err != nil {
			log.Printf("backend: %v", err)
		}
	}()

	a.startNotify()
}

func (a *App) domReady(_ context.Context) {}

func (a *App) shutdown(_ context.Context) {
	if a.notifyProc != nil {
		_ = a.notifyProc.Kill()
	}
	if a.cancelBackend != nil {
		a.cancelBackend()
	}
}

// Ready is called by the frontend once event listeners are registered.
func (a *App) Ready() {
	go a.pollStatus()
}

// ── Workspace CRUD ─────────────────────────────────────────────────────────

// CreateWorkspace creates a new workspace and returns it as JSON.
func (a *App) CreateWorkspace(title string) string {
	st, err := glitchd.OpenStore()
	if err != nil {
		return "{}"
	}
	ws, err := st.CreateWorkspace(a.ctx, title, time.Now().UnixMilli())
	if err != nil {
		return "{}"
	}
	b, _ := json.Marshal(ws)
	return string(b)
}

// ListWorkspaces returns all workspaces as JSON.
func (a *App) ListWorkspaces() string {
	st, err := glitchd.OpenStore()
	if err != nil {
		return "[]"
	}
	wss, err := st.ListWorkspaces(a.ctx)
	if err != nil || wss == nil {
		return "[]"
	}
	b, _ := json.Marshal(wss)
	return string(b)
}

// DeleteWorkspace removes a workspace and all its data.
func (a *App) DeleteWorkspace(id string) {
	st, err := glitchd.OpenStore()
	if err != nil {
		return
	}
	_ = st.DeleteWorkspace(a.ctx, id)
}

// UpdateWorkspaceTitle sets the title of a workspace.
func (a *App) UpdateWorkspaceTitle(id, title string) {
	st, err := glitchd.OpenStore()
	if err != nil {
		return
	}
	_ = st.UpdateWorkspaceTitle(a.ctx, id, title, time.Now().UnixMilli())
}

// AddWorkspaceDirectory opens a native picker and adds the selected dir to the workspace.
func (a *App) AddWorkspaceDirectory(workspaceID string) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Directory to Monitor",
	})
	if err != nil || dir == "" {
		return
	}

	st, err := glitchd.OpenStore()
	if err != nil {
		return
	}
	if err := st.AddWorkspaceDirectory(a.ctx, workspaceID, dir); err != nil {
		log.Printf("add dir: %v", err)
		return
	}

	ws, _ := st.GetWorkspace(a.ctx, workspaceID)
	b, _ := json.Marshal(ws)
	runtime.EventsEmit(a.ctx, "workspace:updated", string(b))
}

// RemoveWorkspaceDirectory removes a directory from a workspace.
func (a *App) RemoveWorkspaceDirectory(workspaceID, dir string) {
	st, err := glitchd.OpenStore()
	if err != nil {
		return
	}
	_ = st.RemoveWorkspaceDirectory(a.ctx, workspaceID, dir)

	ws, _ := st.GetWorkspace(a.ctx, workspaceID)
	b, _ := json.Marshal(ws)
	runtime.EventsEmit(a.ctx, "workspace:updated", string(b))
}

// ── Chat ────────────────────────────────────────────────────────────────────

// AskScoped queries the observer scoped to the workspace's directories.
func (a *App) AskScoped(prompt, workspaceID string) {
	go func() {
		// Get workspace repos for scoping
		var repos []string
		if workspaceID != "" {
			if st, err := glitchd.OpenStore(); err == nil {
				if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
					repos = ws.RepoNames
					_ = st.TouchWorkspace(a.ctx, workspaceID, time.Now().UnixMilli())
				}
			}
		}

		tokenCh := make(chan string, 64)
		go func() {
			for token := range tokenCh {
				runtime.EventsEmit(a.ctx, "chat:chunk", token)
			}
		}()

		var err error
		if len(repos) > 0 {
			err = glitchd.StreamAnswerScoped(a.ctx, prompt, repos, tokenCh)
		} else {
			err = glitchd.StreamAnswer(a.ctx, prompt, tokenCh)
		}

		if err != nil {
			runtime.EventsEmit(a.ctx, "chat:error", err.Error())
			return
		}

		runtime.EventsEmit(a.ctx, "chat:done", nil)
	}()
}

// SaveMessage persists a chat message to the workspace.
func (a *App) SaveMessage(workspaceID, msgJSON string) {
	var msg struct {
		ID        string          `json:"id"`
		Role      string          `json:"role"`
		Blocks    json.RawMessage `json:"blocks"`
		Timestamp int64           `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(msgJSON), &msg); err != nil {
		return
	}
	_ = glitchd.SaveMessage(a.ctx, msg.ID, workspaceID, msg.Role, string(msg.Blocks), msg.Timestamp)
}

// LoadMessages returns all messages for a workspace as JSON.
func (a *App) LoadMessages(workspaceID string) string {
	st, err := glitchd.OpenStore()
	if err != nil {
		return "[]"
	}
	msgs, err := st.GetWorkspaceMessages(a.ctx, workspaceID)
	if err != nil {
		return "[]"
	}
	b, _ := json.Marshal(msgs)
	return string(b)
}

// ── Providers & Agents ──────────────────────────────────────────────────

// ListProviders returns all available providers and models as JSON.
func (a *App) ListProviders() string {
	providers := glitchd.ListProviders()
	b, _ := json.Marshal(providers)
	return string(b)
}

// ListAgents returns discovered agents/skills for the active workspace dirs.
func (a *App) ListAgents(workspaceID string) string {
	var dirs []string
	if workspaceID != "" {
		if st, err := glitchd.OpenStore(); err == nil {
			if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
				dirs = ws.Directories
			}
		}
	}
	agents := glitchd.ListAgents(dirs)
	if agents == nil {
		return "[]"
	}
	b, _ := json.Marshal(agents)
	return string(b)
}

// AskProvider sends a prompt to a chosen provider/model with full glitch context injected.
// agentPath is optional — if set, the agent's instructions are prepended.
func (a *App) AskProvider(providerID, model, prompt, workspaceID, agentPath string) {
	go func() {
		// Build context from workspace
		var dirs []string
		var agents []glitchd.AgentInfo
		var pipes []glitchd.PipelineInfo

		if workspaceID != "" {
			if st, err := glitchd.OpenStore(); err == nil {
				if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
					dirs = ws.Directories
				}
			}
			agents = glitchd.ListAgents(dirs)
			pipes = glitchd.DiscoverWorkspacePipelines(dirs)
		}

		systemCtx := glitchd.BuildSystemContext(dirs, agents, pipes)

		tokenCh := make(chan string, 64)
		go func() {
			for token := range tokenCh {
				runtime.EventsEmit(a.ctx, "chat:chunk", token)
			}
		}()

		err := glitchd.StreamPrompt(a.ctx, glitchd.StreamPromptOpts{
			ProviderID: providerID,
			Model:      model,
			Prompt:     prompt,
			SystemCtx:  systemCtx,
			AgentPath:  agentPath,
		}, tokenCh)

		if err != nil {
			runtime.EventsEmit(a.ctx, "chat:error", err.Error())
			return
		}

		runtime.EventsEmit(a.ctx, "chat:done", nil)
	}()
}

// ── Pipelines ───────────────────────────────────────────────────────────

// ListPipelines returns discovered pipelines from the active workspace's directories.
func (a *App) ListPipelines(workspaceID string) string {
	var dirs []string
	if workspaceID != "" {
		if st, err := glitchd.OpenStore(); err == nil {
			if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
				dirs = ws.Directories
			}
		}
	}
	pipes := glitchd.DiscoverWorkspacePipelines(dirs)
	if pipes == nil {
		return "[]"
	}
	b, _ := json.Marshal(pipes)
	return string(b)
}

// RunPipeline executes a pipeline and streams output as chat events.
func (a *App) RunPipeline(pipelinePath, input string) {
	go func() {
		tokenCh := make(chan string, 64)
		go func() {
			for token := range tokenCh {
				runtime.EventsEmit(a.ctx, "chat:chunk", token)
			}
		}()

		if err := glitchd.RunPipeline(a.ctx, pipelinePath, input, tokenCh); err != nil {
			runtime.EventsEmit(a.ctx, "chat:error", err.Error())
			return
		}
		runtime.EventsEmit(a.ctx, "chat:done", nil)
	}()
}

// SavePipeline saves pipeline YAML to a project directory.
func (a *App) SavePipeline(projectDir, name, yamlContent string) string {
	path, err := glitchd.SavePipeline(projectDir, name, yamlContent)
	if err != nil {
		return ""
	}
	return path
}

// Doctor runs health checks and streams the report.
func (a *App) Doctor() {
	go func() {
		checks := glitchd.Doctor(a.ctx)
		report := glitchd.DoctorReport(checks)
		runtime.EventsEmit(a.ctx, "chat:chunk", report)
		runtime.EventsEmit(a.ctx, "chat:done", nil)
	}()
}

// ── Status polling ──────────────────────────────────────────────────────────

func (a *App) pollStatus() {
	check := func() {
		runtime.EventsEmit(a.ctx, "status:all", map[string]bool{
			"ollama":        pingHTTP("http://localhost:11434"),
			"elasticsearch": pingHTTP("http://localhost:9200"),
			"busd":          pingUnix(busdSocket()),
		})
	}
	check()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

// ── Notify ──────────────────────────────────────────────────────────────────

func (a *App) startNotify() {
	home, _ := os.UserHomeDir()
	binary := filepath.Join(home, ".local", "bin", "glitch-notify")
	if _, err := os.Stat(binary); err != nil {
		return
	}
	_ = exec.Command("pkill", "-f", "glitch-notify").Run()
	cmd := exec.Command(binary)
	if err := cmd.Start(); err != nil {
		return
	}
	a.notifyProc = cmd.Process
	go func() { _ = cmd.Wait() }()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func pingHTTP(url string) bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func pingUnix(path string) bool {
	if path == "" {
		return false
	}
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func busdSocket() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "glitch", "bus.sock")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "glitch", "bus.sock")
}
