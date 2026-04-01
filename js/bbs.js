/* ─────────────────────────────────────────────────────────────
   ORCAI ABBS  —  bbs.js
   Hex dump canvas · Typewriter · KeyboardRouter · ANSI pipeline demo
   ───────────────────────────────────────────────────────────── */

// ── KeyboardRouter ────────────────────────────────────────────
var KeyboardRouter = (function() {
  var activeScreen = 'home';
  var screenHandlers = {};
  var screenOrder = ['home','about','getting-started','plugins','pipelines','changelog','themes','labs','docs'];

  function switchScreen(id) {
    document.querySelectorAll('.screen').forEach(function(el) { el.classList.remove('active'); });
    var target = document.querySelector('[data-screen="' + id + '"]');
    if (target) target.classList.add('active');
    // update nav active
    document.querySelectorAll('[data-nav-screen]').forEach(function(el) {
      el.classList.toggle('active', el.dataset.navScreen === id);
    });
    // update status bar
    var csEl = document.querySelector('.current-screen');
    if (csEl) csEl.textContent = id.toUpperCase().replace(/-/g,' ');
    activeScreen = id;
    // call onEnter
    if (screenHandlers[id] && screenHandlers[id]._onEnter) screenHandlers[id]._onEnter();
  }

  function register(screenId, handlers) {
    screenHandlers[screenId] = handlers;
  }

  function toggleHelp() {
    var overlay = document.querySelector('.help-overlay');
    if (overlay) overlay.classList.toggle('open');
  }

  function dispatch(e) {
    if (e.target && (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA')) return;
    var overlay = document.querySelector('.help-overlay');
    if (overlay && overlay.classList.contains('open')) {
      if (e.key === '?' || e.key === 'Escape' || e.key === 'F1') { e.preventDefault(); toggleHelp(); }
      return;
    }
    var idx = parseInt(e.key);
    if (idx >= 1 && idx <= screenOrder.length) { e.preventDefault(); switchScreen(screenOrder[idx-1]); return; }
    switch (e.key) {
      case '?': case 'F1': e.preventDefault(); toggleHelp(); break;
      case 'q': case 'Escape':
        document.body.style.transition = 'opacity 0.5s';
        document.body.style.opacity = '0';
        setTimeout(function() { document.body.style.opacity = '1'; }, 2500);
        break;
      default:
        var h = screenHandlers[activeScreen];
        if (h && h[e.key]) h[e.key](e);
    }
  }

  return {
    switchScreen: switchScreen,
    register: register,
    dispatch: dispatch,
    toggleHelp: toggleHelp,
    getActive: function() { return activeScreen; }
  };
})();

// ── Hex dump background canvas ────────────────────────────────
function initHexCanvas() {
  var canvas = document.getElementById('hexbg');
  if (!canvas) return;

  var ctx = canvas.getContext('2d');
  var animId = null;

  function resize() {
    canvas.width  = window.innerWidth;
    canvas.height = window.innerHeight;
  }
  resize();
  window.addEventListener('resize', function() { resize(); setup(); });

  var COL_W  = 28;
  var ROW_H  = 18;
  var COLS, ROWS, offsets, speeds, grid;

  function hexByte() {
    return Math.floor(Math.random() * 256).toString(16).padStart(2, '0').toUpperCase();
  }

  function setup() {
    COLS = Math.ceil(canvas.width  / COL_W) + 2;
    ROWS = Math.ceil(canvas.height / ROW_H) + 3;

    offsets = Array.from({ length: COLS }, function() { return Math.random() * ROWS * ROW_H; });
    speeds  = Array.from({ length: COLS }, function() { return 0.25 + Math.random() * 0.45; });

    var GRID_H = ROWS * 4;
    grid = Array.from({ length: GRID_H }, function() {
      return Array.from({ length: COLS }, hexByte);
    });
  }

  setup();

  function draw() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.font      = '12px "Share Tech Mono", monospace';
    ctx.fillStyle = 'rgba(98, 114, 164, 0.12)';

    for (var col = 0; col < COLS; col++) {
      offsets[col] = (offsets[col] + speeds[col]) % (grid.length * ROW_H);

      var startRow = Math.floor(offsets[col] / ROW_H);
      var subPx    = offsets[col] % ROW_H;

      for (var row = 0; row <= ROWS + 1; row++) {
        var y       = row * ROW_H - subPx;
        var gridRow = (startRow + row) % grid.length;
        ctx.fillText(grid[gridRow][col], col * COL_W, y);
      }
    }

    animId = requestAnimationFrame(draw);
  }

  if (animId) cancelAnimationFrame(animId);
  draw();
}

// ── Typewriter effect ─────────────────────────────────────────
function typewriter(el, lines, charDelay, pauseDelay) {
  charDelay  = charDelay  || 40;
  pauseDelay = pauseDelay || 1800;
  if (!el) return;

  var lineIdx  = 0;
  var charIdx  = 0;
  var erasing  = false;

  function tick() {
    var line = lines[lineIdx];

    if (!erasing) {
      if (charIdx <= line.length) {
        el.textContent = '> ' + line.slice(0, charIdx) + '\u2588';
        charIdx++;
        setTimeout(tick, charDelay + Math.random() * 18);
      } else {
        erasing = true;
        setTimeout(tick, pauseDelay);
      }
    } else {
      if (charIdx > 0) {
        charIdx--;
        el.textContent = '> ' + line.slice(0, charIdx) + '\u2588';
        setTimeout(tick, charDelay * 0.5);
      } else {
        erasing  = false;
        lineIdx  = (lineIdx + 1) % lines.length;
        charIdx  = 0;
        setTimeout(tick, 300);
      }
    }
  }

  tick();
}

// ── ANSI logo color cycling ────────────────────────────────────
function colorLogo() {
  var logo = document.querySelector('.ansi-logo');
  if (!logo) return;

  var colors = [
    '#bd93f9',
    '#ff79c6',
    '#8be9fd',
    '#50fa7b',
    '#f1fa8c',
    '#ff79c6',
  ];

  var rawLines = logo.textContent.split('\n');
  var fragment = document.createDocumentFragment();

  rawLines.forEach(function(line, i) {
    var span = document.createElement('span');
    span.style.display = 'block';
    if (line.trim()) {
      span.style.color = colors[i % colors.length];
    }
    span.textContent = line;
    fragment.appendChild(span);
  });

  while (logo.firstChild) logo.removeChild(logo.firstChild);
  logo.appendChild(fragment);
}

// ── Clock ──────────────────────────────────────────────────────
function initClock() {
  function tick() {
    var el = document.querySelector('.clock');
    if (el) {
      var now = new Date();
      el.textContent = now.toTimeString().slice(0,8);
    }
  }
  tick();
  setInterval(tick, 1000);
}

// ── Copy buttons ───────────────────────────────────────────────
function initCopyButtons() {
  document.querySelectorAll('pre.code-block').forEach(function(pre) {
    var btn       = document.createElement('button');
    btn.className = 'copy-btn';
    btn.textContent = '[COPY]';

    btn.addEventListener('click', function() {
      var raw  = pre.innerText || pre.textContent;
      var text = raw.replace(/\[COPY\]|\[COPIED\]/g, '').trim();

      function success() {
        btn.textContent  = '[COPIED]';
        btn.style.color  = 'var(--green)';
        setTimeout(function() {
          btn.textContent = '[COPY]';
          btn.style.color = '';
        }, 2000);
      }

      if (navigator.clipboard) {
        navigator.clipboard.writeText(text).then(success).catch(function() {
          fallbackCopy(text);
          success();
        });
      } else {
        fallbackCopy(text);
        success();
      }
    });

    pre.appendChild(btn);
  });
}

function fallbackCopy(text) {
  var ta       = document.createElement('textarea');
  ta.value     = text;
  ta.style.position = 'fixed';
  ta.style.opacity  = '0';
  document.body.appendChild(ta);
  ta.select();
  try { document.execCommand('copy'); } catch(e) {}
  document.body.removeChild(ta);
}

// ── Pipeline example switcher ──────────────────────────────────
var PipelineExamples = (function() {
  var current = 0;

  var examples = [
    // 0: code-review
    [
      '# code-review — diff summarizer + issue extractor',
      '# runs on every PR: claude reads the diff, jq filters',
      '# high-severity issues, writes a timestamped report.',
      '',
      'name: code-review',
      'description: Summarize a PR diff and extract high-severity issues',
      '',
      'steps:',
      '  - id: summarize',
      '    executor: claude',
      '    model: claude-sonnet-4-6',
      '    prompt: |',
      '      You are a senior engineer reviewing a pull request.',
      '      Summarize the key changes, flag any risks, and note',
      '      anything that needs a second look.',
      '      Diff: {{diff}}',
      '',
      '  - id: extract-issues',
      '    plugin: jq',
      '    vars:',
      '      filter: "[.issues[]? | select(.severity==\"high\")]"',
      '',
      '  - id: write-report',
      '    plugin: write',
      '    vars:',
      '      path: "./reports/review-{{date}}.json"'
    ].join('\n'),

    // 1: brain-feedback-loop
    [
      '# brain-feedback-loop — local → cloud knowledge chain',
      '# qwen2.5 analyzes cheaply and writes an insight note.',
      '# claude reads that note and synthesizes a recommendation.',
      '',
      'name: brain-feedback-loop',
      'description: Local model builds context, cloud model acts on it',
      '',
      'use_brain: true',
      '',
      'steps:',
      '  - id: analyze',
      '    executor: ollama',
      '    model: qwen2.5-coder:latest',
      '    write_brain: true',
      '    prompt: |',
      '      Analyze the codebase in ./src. Identify the single',
      '      most important architectural insight. Be specific.',
      '      Start your response with "INSIGHT:"',
      '',
      '  - id: synthesize',
      '    executor: claude',
      '    model: claude-haiku-4-5-20251001',
      '    use_brain: true',
      '    prompt: |',
      '      Given the architectural insight above, write a',
      '      concrete refactor plan. Include file names.',
      '      Keep it under 200 words.'
    ].join('\n'),

    // 2: gh-triage
    [
      '# gh-triage — automated issue triage + weekly digest',
      '# fetches open issues from GitHub, runs local analysis,',
      '# saves a prioritized markdown report to disk.',
      '',
      'name: gh-triage',
      'description: Weekly issue triage with Ollama + brain context',
      '',
      'steps:',
      '  - id: fetch-issues',
      '    plugin: gh',
      '    vars:',
      '      args: "issue list --json number,title,body,labels --limit 30"',
      '',
      '  - id: fetch-recent-prs',
      '    plugin: gh',
      '    vars:',
      '      args: "pr list --state merged --json number,title --limit 10"',
      '',
      '  - id: triage',
      '    executor: ollama',
      '    model: llama3.2:latest',
      '    write_brain: true',
      '    prompt: |',
      '      Issues: {{steps.fetch-issues.output}}',
      '      Recent merged PRs: {{steps.fetch-recent-prs.output}}',
      '      Rank the top 5 open issues by impact. Explain briefly.',
      '',
      '  - id: save',
      '    plugin: write',
      '    vars:',
      '      path: "./triage/{{date}}.md"'
    ].join('\n')
  ];

  var twTimer = null;

  function show(idx) {
    current = idx;
    var el = document.getElementById('pipeline-example');
    if (!el) return;
    // update tab active state
    document.querySelectorAll('.pipeline-tab').forEach(function(btn, i) {
      btn.classList.toggle('active', i === idx);
    });
    // typewriter animation
    if (twTimer) clearTimeout(twTimer);
    el.textContent = '';
    var text = examples[idx];
    var i = 0;
    function tick() {
      if (i < text.length) {
        el.textContent = text.slice(0, i + 1);
        i++;
        twTimer = setTimeout(tick, i % 40 === 0 ? 100 : 14);
      }
    }
    tick();
  }

  function next() { show((current + 1) % examples.length); }

  return { show: show, next: next, current: function() { return current; } };
})();

// ── Boot ───────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', function() {
  initHexCanvas();
  colorLogo();
  initClock();
  initCopyButtons();

  var twEl = document.getElementById('typewriter');
  if (twEl) {
    typewriter(twEl, [
      'orchestrate your agents.',
      'build pipelines in YAML.',
      'run in tmux. stay in the terminal.',
      'plugins are just CLI tools.',
      'chain claude → jq → write. done.'
    ]);
  }

  // Register pipelines screen handler
  KeyboardRouter.register('pipelines', {
    'Tab': function(e) { e.preventDefault(); PipelineExamples.next(); },
    'r':   function() { PipelineExamples.show(PipelineExamples.current()); },
    'R':   function() { PipelineExamples.show(PipelineExamples.current()); },
    _onEnter: function() { PipelineExamples.show(0); }
  });

  // keyboard router
  document.addEventListener('keydown', KeyboardRouter.dispatch);

  // activate home screen by default
  KeyboardRouter.switchScreen('home');
});
