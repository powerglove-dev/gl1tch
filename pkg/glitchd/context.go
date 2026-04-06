package glitchd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildSystemContext assembles the glitch-aware system prompt that gets
// injected into every provider call. This teaches the provider about
// available tools, workflow format, executors, and workspace context.
func BuildSystemContext(dirs []string, agents []AgentInfo, workflows []WorkflowInfo) string {
	var sb strings.Builder

	sb.WriteString("You are gl1tch, a developer assistant. You have access to the following capabilities.\n\n")

	// Available executors
	sb.WriteString("## Available Executors\n")
	sb.WriteString("When building workflows, these executors are available:\n")
	providers := ListProviders()
	for _, p := range providers {
		models := make([]string, 0, len(p.Models))
		for _, m := range p.Models {
			models = append(models, m.ID)
		}
		sb.WriteString(fmt.Sprintf("- **%s** (%s): models [%s]\n", p.ID, p.Label, strings.Join(models, ", ")))
	}
	sb.WriteString("- **shell**: execute system commands (git, npm, curl, etc.)\n\n")

	// Workflow format
	sb.WriteString("## Workflow YAML Format\n")
	sb.WriteString("Workflows are YAML files in <workspace>/.glitch/workflows/<name>.workflow.yaml:\n")
	sb.WriteString("```yaml\n")
	sb.WriteString("name: my-workflow\n")
	sb.WriteString("version: \"1\"\n")
	sb.WriteString("description: \"What this workflow does\"\n")
	sb.WriteString("\n")
	sb.WriteString("steps:\n")
	sb.WriteString("  - id: gather\n")
	sb.WriteString("    executor: shell\n")
	sb.WriteString("    vars:\n")
	sb.WriteString("      cmd: \"git log --oneline -10\"\n")
	sb.WriteString("\n")
	sb.WriteString("  - id: analyze\n")
	sb.WriteString("    executor: claude\n")
	sb.WriteString("    model: claude-sonnet-4-6\n")
	sb.WriteString("    needs: [gather]\n")
	sb.WriteString("    prompt: |\n")
	sb.WriteString("      Analyze these commits:\n")
	sb.WriteString("      {{get \"step.gather.data.value\" .}}\n")
	sb.WriteString("```\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- Shell steps own data fetching (git, curl, API calls)\n")
	sb.WriteString("- LLM steps own reasoning and formatting\n")
	sb.WriteString("- Use `needs:` to express step dependencies\n")
	sb.WriteString("- Use `{{get \"step.<id>.data.value\" .}}` to reference previous step output\n")
	sb.WriteString("- Use `{{param.input}}` for user-provided input\n\n")

	// Cron scheduling
	sb.WriteString("## Cron Scheduling\n")
	sb.WriteString("Workflows can be scheduled via cron.yaml:\n")
	sb.WriteString("```yaml\n")
	sb.WriteString("- name: daily-digest\n")
	sb.WriteString("  schedule: \"0 9 * * 1-5\"  # weekdays at 9am\n")
	sb.WriteString("  kind: workflow\n")
	sb.WriteString("  target: git-digest\n")
	sb.WriteString("  timeout: 15m\n")
	sb.WriteString("```\n\n")

	// Workspace directories
	if len(dirs) > 0 {
		sb.WriteString("## Workspace Directories\n")
		sb.WriteString("The user is working with these projects:\n")
		for _, d := range dirs {
			sb.WriteString(fmt.Sprintf("- `%s` (%s)\n", filepath.Base(d), d))
		}
		sb.WriteString("\n")
	}

	// Available agents
	skills := filterByKind(agents, "skill")
	agentList := filterByKind(agents, "agent")

	if len(skills) > 0 {
		sb.WriteString("## Available Skills\n")
		for _, a := range skills {
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", a.Invoke, a.Description))
		}
		sb.WriteString("\n")
	}

	if len(agentList) > 0 {
		sb.WriteString("## Available Agents\n")
		for _, a := range agentList {
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", a.Invoke, a.Description))
		}
		sb.WriteString("\n")
	}

	// Existing workflows
	if len(workflows) > 0 {
		sb.WriteString("## Existing Workflows\n")
		for _, w := range workflows {
			sb.WriteString(fmt.Sprintf("- **%s** (%s) — %s\n", w.Name, w.Workspace, w.Description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Response Rules\n")
	sb.WriteString("- When the user asks to build a workflow, output valid workflow YAML in a code block\n")
	sb.WriteString("- When the user asks about their projects, reference the workspace directories above\n")
	sb.WriteString("- Be concise and direct\n")
	sb.WriteString("- If you don't know something, say so — never fabricate\n")

	return sb.String()
}

// BuildAgentPrompt reads an agent's instructions and prepends them to the user prompt.
func BuildAgentPrompt(agentPath, userPrompt string) string {
	data, err := os.ReadFile(agentPath)
	if err != nil {
		return userPrompt
	}
	return string(data) + "\n\n---\n\nUser request:\n" + userPrompt
}

func filterByKind(agents []AgentInfo, kind string) []AgentInfo {
	var out []AgentInfo
	for _, a := range agents {
		if a.Kind == kind {
			out = append(out, a)
		}
	}
	return out
}
