#!/usr/bin/env bash
# gen-stats.sh — generate site/public/project-stats.json from git history.
# Works on macOS and Linux (uses python3 for all date math).
# Usage: bash scripts/gen-stats.sh
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
OUT="${REPO_ROOT}/site/public/project-stats.json"
ACTIVITY="${REPO_ROOT}/site/public/activity-snapshot.jsonl"

python3 - "$OUT" "$ACTIVITY" <<'PYEOF'
import sys, json, subprocess, os
from datetime import datetime, timezone, timedelta

out_path  = sys.argv[1]
act_path  = sys.argv[2]

# ── git log ────────────────────────────────────────────────────────────────────
raw = subprocess.check_output(
    ['git', 'log', '--format=%ai|%s', '--reverse'], text=True
).strip()
if not raw:
    print('gen-stats: no commits found', file=sys.stderr)
    sys.exit(1)

commits = []
for line in raw.splitlines():
    if '|' not in line:
        continue
    ts_str, msg = line.split('|', 1)
    ts = datetime.fromisoformat(ts_str.strip())
    # normalise to UTC-aware
    if ts.tzinfo is None:
        ts = ts.replace(tzinfo=timezone.utc)
    commits.append({'ts': ts, 'msg': msg.strip()})

now = datetime.now(timezone.utc)
first_ts = commits[0]['ts']
project_age_days = (now - first_ts).days
total_commits    = len(commits)

# commits this week
week_ago = now - timedelta(days=7)
commits_this_week = sum(1 for c in commits if c['ts'] >= week_ago)

# ── velocity trend (first half vs second half) ─────────────────────────────────
mid = total_commits // 2
first_half  = commits[:mid]
second_half = commits[mid:]

def cpd(cs):
    if len(cs) < 2:
        return 0.0
    span = (cs[-1]['ts'] - cs[0]['ts']).total_seconds() / 86400
    return len(cs) / span if span > 0 else 0.0

cpd_early  = cpd(first_half)
cpd_recent = cpd(second_half)
if cpd_early > 0:
    mult = cpd_recent / cpd_early
    velocity_trend = f'+{mult:.1f}x' if mult >= 1 else f'-{1/mult:.1f}x'
else:
    velocity_trend = '+1.0x'

# ── feat / fix ratios ──────────────────────────────────────────────────────────
def ratio(cs, prefix):
    if not cs:
        return 0.0
    return round(sum(1 for c in cs if c['msg'].lower().startswith(prefix)) / len(cs), 2)

feat_ratio_early  = ratio(first_half,  'feat')
feat_ratio_recent = ratio(second_half, 'feat')
fix_ratio_early   = ratio(first_half,  'fix')
fix_ratio_recent  = ratio(second_half, 'fix')

# ── median inter-commit gap: first third vs last third ─────────────────────────
def median_gap_hrs(cs):
    if len(cs) < 2:
        return None
    gaps = sorted(
        (cs[i]['ts'] - cs[i-1]['ts']).total_seconds() / 3600
        for i in range(1, len(cs))
    )
    return round(gaps[len(gaps) // 2], 1)

third = max(2, total_commits // 3)
median_gap_early  = median_gap_hrs(commits[:third])
median_gap_recent = median_gap_hrs(commits[-third:])

if median_gap_early and median_gap_recent and median_gap_early > 0:
    pct = (median_gap_recent - median_gap_early) / median_gap_early * 100
    feedback_loop_delta = f'{pct:+.0f}%'
else:
    feedback_loop_delta = None

# ── activity snapshot (optional) ──────────────────────────────────────────────
pipeline_runs = pipeline_success_rate = brain_events = None
if os.path.exists(act_path):
    started = finished = failed = scheduled = 0
    with open(act_path) as f:
        for line in f:
            try:
                k = json.loads(line).get('kind', '')
                if k == 'pipeline_started':  started   += 1
                elif k == 'pipeline_finished': finished += 1
                elif k == 'pipeline_failed':   failed   += 1
                elif k == 'schedule_fired':    scheduled += 1
            except Exception:
                pass
    pipeline_runs = started
    total_out = finished + failed
    pipeline_success_rate = round(finished / total_out, 2) if total_out > 0 else None
    brain_events = scheduled

# ── write ──────────────────────────────────────────────────────────────────────
stats = {
    'generated_at':         now.strftime('%Y-%m-%d'),
    'project_age_days':     project_age_days,
    'total_commits':        total_commits,
    'commits_this_week':    commits_this_week,
    'velocity_trend':       velocity_trend,
    'feat_ratio_early':     feat_ratio_early,
    'feat_ratio_recent':    feat_ratio_recent,
    'fix_ratio_early':      fix_ratio_early,
    'fix_ratio_recent':     fix_ratio_recent,
    'median_gap_early_hrs': median_gap_early,
    'median_gap_recent_hrs':median_gap_recent,
    'feedback_loop_delta':  feedback_loop_delta,
    'pipeline_runs':        pipeline_runs,
    'pipeline_success_rate':pipeline_success_rate,
    'brain_events':         brain_events,
}

os.makedirs(os.path.dirname(out_path), exist_ok=True)
with open(out_path, 'w') as f:
    json.dump(stats, f, indent=2)
    f.write('\n')

print(f'gen-stats: wrote {out_path}')
for k, v in stats.items():
    if v is not None:
        print(f'  {k}: {v}')
PYEOF
