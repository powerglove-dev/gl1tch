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
		var pipes []glitchd.WorkflowInfo

		if workspaceID != "" {
			if st, err := glitchd.OpenStore(); err == nil {
				if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
					dirs = ws.Directories
				}
			}
			agents = glitchd.ListAgents(dirs)
			pipes = glitchd.DiscoverWorkspaceWorkflows(dirs)
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

// ── Prompts ────────────────────────────────────────────────────────────

// ListPrompts returns all saved prompts as JSON.
func (a *App) ListPrompts() string {
	return glitchd.ListAllPrompts(a.ctx)
}

// CreatePrompt saves a new prompt and returns it as JSON.
func (a *App) CreatePrompt(title, body, modelSlug string) string {
	return glitchd.CreatePrompt(a.ctx, title, body, modelSlug)
}

// DeletePrompt removes a prompt by ID.
func (a *App) DeletePrompt(id int64) {
	glitchd.DeletePromptByID(a.ctx, id)
}

// GetWorkflowFileDetails returns metadata about a workflow YAML file on disk:
// description and the list of inner steps with their executor and a short
// prompt preview. Used by the step editor in the desktop builder so users
// can see what a workflow does without leaving the chat.
func (a *App) GetWorkflowFileDetails(path string) string {
	return glitchd.GetWorkflowFileDetails(path)
}

// ── Chain execution ─────────────────────────────────────────────────────

// RunChain executes a builder chain (JSON-encoded list of ChainStep) sequentially.
// Each step's output flows into the next via {{ steps.step-N.value }} refs.
// userText is appended as a final implicit prompt step if non-empty.
func (a *App) RunChain(stepsJSON, userText, workspaceID, defaultProvider, defaultModel string) {
	go func() {
		// Build system context from workspace.
		var dirs []string
		var agents []glitchd.AgentInfo
		var pipes []glitchd.WorkflowInfo
		if workspaceID != "" {
			if st, err := glitchd.OpenStore(); err == nil {
				if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
					dirs = ws.Directories
				}
			}
			agents = glitchd.ListAgents(dirs)
			pipes = glitchd.DiscoverWorkspaceWorkflows(dirs)
		}
		systemCtx := glitchd.BuildSystemContext(dirs, agents, pipes)

		// Start clarification poller for the duration of the run.
		clarifyCtx, clarifyCancel := context.WithCancel(context.Background())
		go a.pollClarifications(clarifyCtx)

		tokenCh := make(chan string, 64)
		go func() {
			for token := range tokenCh {
				runtime.EventsEmit(a.ctx, "chat:chunk", token)
			}
		}()

		err := glitchd.RunChain(a.ctx, glitchd.RunChainOpts{
			StepsJSON:       stepsJSON,
			UserText:        userText,
			WorkspaceID:     workspaceID,
			DefaultProvider: defaultProvider,
			DefaultModel:    defaultModel,
			SystemCtx:       systemCtx,
		}, tokenCh)
		clarifyCancel()

		if err != nil {
			runtime.EventsEmit(a.ctx, "chat:error", err.Error())
			return
		}
		runtime.EventsEmit(a.ctx, "chat:done", nil)
	}()
}

// ── Chat Workflows ─────────────────────────────────────────────────────

// ListChatWorkflows returns saved workflows for a workspace as JSON.
func (a *App) ListChatWorkflows(workspaceID string) string {
	return glitchd.ListChatWorkflows(a.ctx, workspaceID)
}

// SaveChatWorkflow saves a new workflow and returns it as JSON.
func (a *App) SaveChatWorkflow(workspaceID, name, stepsJSON string) string {
	return glitchd.SaveChatWorkflow(a.ctx, workspaceID, name, stepsJSON)
}

// UpdateChatWorkflow modifies an existing workflow.
func (a *App) UpdateChatWorkflow(id int64, name, stepsJSON string) {
	glitchd.UpdateChatWorkflow(a.ctx, id, name, stepsJSON)
}

// DeleteChatWorkflow removes a workflow.
func (a *App) DeleteChatWorkflow(id int64) {
	glitchd.DeleteChatWorkflow(a.ctx, id)
}

// ── Clarification ──────────────────────────────────────────────────────

// AnswerClarification writes the user's answer for a pending clarification.
func (a *App) AnswerClarification(runID, answer string) {
	glitchd.AnswerClarification(runID, answer)
}

// pollClarifications polls the DB for pending clarification requests during
// pipeline runs and forwards them to the frontend as Wails events.
func (a *App) pollClarifications(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	notified := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reqs, err := glitchd.LoadPendingClarifications()
			if err != nil {
				continue
			}
			for _, req := range reqs {
				if notified[req.RunID] {
					continue
				}
				notified[req.RunID] = true
				runtime.EventsEmit(a.ctx, "chat:clarify", map[string]string{
					"run_id":   req.RunID,
					"step_id":  req.StepID,
					"question": req.Question,
				})
			}
		}
	}
}

// ── Workflows ───────────────────────────────────────────────────────────

// ListWorkflows returns discovered workflows from the active workspace's directories.
func (a *App) ListWorkflows(workspaceID string) string {
	var dirs []string
	if workspaceID != "" {
		if st, err := glitchd.OpenStore(); err == nil {
			if ws, err := st.GetWorkspace(a.ctx, workspaceID); err == nil {
				dirs = ws.Directories
			}
		}
	}
	workflows := glitchd.DiscoverWorkspaceWorkflows(dirs)
	if workflows == nil {
		return "[]"
	}
	b, _ := json.Marshal(workflows)
	return string(b)
}

// RunWorkflow executes a workflow and streams output as chat events.
func (a *App) RunWorkflow(workflowPath, input string) {
	go func() {
		// Start polling for clarification requests during this workflow run.
		clarifyCtx, clarifyCancel := context.WithCancel(context.Background())
		go a.pollClarifications(clarifyCtx)

		tokenCh := make(chan string, 64)
		go func() {
			for token := range tokenCh {
				runtime.EventsEmit(a.ctx, "chat:chunk", token)
			}
		}()

		err := glitchd.RunWorkflow(a.ctx, workflowPath, input, tokenCh)
		clarifyCancel()

		if err != nil {
			runtime.EventsEmit(a.ctx, "chat:error", err.Error())
			return
		}
		runtime.EventsEmit(a.ctx, "chat:done", nil)
	}()
}

// SaveWorkflow saves workflow YAML to a workspace directory.
func (a *App) SaveWorkflow(workspaceDir, name, yamlContent string) string {
	path, err := glitchd.SaveWorkflow(workspaceDir, name, yamlContent)
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
