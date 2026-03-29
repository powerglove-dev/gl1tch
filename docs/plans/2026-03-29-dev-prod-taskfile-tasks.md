# Dev and Prod Taskfile Tasks Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `dev` and `prod` tasks to `Taskfile.yml` for clean-slate dev iteration and daily-driver prod startup.

**Architecture:** Two new flat tasks appended to the Run tasks section of `Taskfile.yml`. No new files, no new dependencies. `dev` cleans everything and builds from source; `prod` installs from source and runs non-destructively.

**Tech Stack:** Task v3 (`Taskfile.yml`), Go toolchain (`go build`, `go install`), bash, tmux.

---

### Task 1: Add `dev` task

**Files:**
- Modify: `Taskfile.yml` (Run tasks section, after `run:clean`)

**Step 1: Read the current Taskfile to get exact insertion point**

Open `Taskfile.yml` and note the line number where `run:clean` ends and the `# Debug tasks` comment begins.

**Step 2: Insert the `dev` task between `run:clean` and the debug section comment**

```yaml
  dev:
    desc: "New-user clean build: backup+wipe config/data/binaries, rebuild from source, run ./bin/orcai"
    cmds:
      - -tmux detach-client -s orcai 2>/dev/null
      - -tmux kill-session -t orcai 2>/dev/null
      - -tmux kill-session -t orcai-cron 2>/dev/null
      - |
        if [ -d ~/.config/orcai ]; then
          cp -r ~/.config/orcai ~/.config/orcai.bak.$(date +%Y%m%d%H%M%S)
          ls -dt ~/.config/orcai.bak.* 2>/dev/null | tail -n +6 | xargs rm -rf
        fi
      - |
        if [ -d ~/.local/share/orcai ]; then
          cp -r ~/.local/share/orcai ~/.local/share/orcai.bak.$(date +%Y%m%d%H%M%S)
          ls -dt ~/.local/share/orcai.bak.* 2>/dev/null | tail -n +6 | xargs rm -rf
        fi
      - rm -rf ~/.config/orcai ~/.local/share/orcai
      - rm -f bin/{{.BINARY}} bin/{{.BINARY}}-debug
      - go build -o bin/{{.BINARY}} .
      - ./bin/{{.BINARY}}
```

**Step 3: Verify the task appears in task --list**

Run: `task --list`
Expected: `dev` appears with description "New-user clean build: backup+wipe config/data/binaries, rebuild from source, run ./bin/orcai"

**Step 4: Commit**

```bash
git add Taskfile.yml
git commit -m "feat(taskfile): add dev task — clean-slate build with backup"
```

---

### Task 2: Add `prod` task

**Files:**
- Modify: `Taskfile.yml` (after `dev` task, before debug section comment)

**Step 1: Insert the `prod` task after `dev`**

```yaml
  prod:
    desc: "Daily driver: install fresh build from source, run non-destructively (state preserved)"
    cmds:
      - go install .
      - |
        if [ ! -L ~/.local/bin/{{.BINARY}} ]; then
          rm -f ~/.local/bin/{{.BINARY}}
          ln -sf $(go env GOPATH)/bin/{{.BINARY}} ~/.local/bin/{{.BINARY}}
        fi
      - "{{.BINARY}}"
```

**Step 2: Verify the task appears in task --list**

Run: `task --list`
Expected: `prod` appears with description "Daily driver: install fresh build from source, run non-destructively (state preserved)"

**Step 3: Commit**

```bash
git add Taskfile.yml
git commit -m "feat(taskfile): add prod task — install from source, run as daily driver"
```

---

### Task 3: Smoke-test `dev` task (dry run)

**Step 1: Verify the backup+wipe logic without running orcai**

Temporarily confirm the shell logic is correct by checking if the backup dirs would be created:

```bash
# Confirm config dir exists
ls ~/.config/orcai/
# Confirm data dir exists
ls ~/.local/share/orcai/
```

**Step 2: Review the full Taskfile for correctness**

Run: `task --list`
Expected: All existing tasks still present, plus `dev` and `prod`.

**Step 3: Confirm task --summary for both new tasks**

Run: `task --summary dev && task --summary prod`
Expected: Full command sequences printed for both tasks with no YAML parse errors.
