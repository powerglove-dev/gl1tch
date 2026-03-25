/* ─────────────────────────────────────────────────────────────
   ORCAI ABS  —  bbs.js
   Hex dump canvas · Typewriter · Keyboard nav · Copy buttons
   ANSI logo color cycling
   ───────────────────────────────────────────────────────────── */

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

  // Clear and repopulate using safe DOM methods only
  while (logo.firstChild) logo.removeChild(logo.firstChild);
  logo.appendChild(fragment);
}

// ── Keyboard navigation (index page only) ────────────────────
function initKeyboardNav() {
  document.addEventListener('keydown', function(e) {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

    switch (e.key) {
      case 'Enter':
        window.location.href = '/orcai/getting-started';
        break;
      case 'p':
      case 'P':
        window.location.href = '/orcai/plugins';
        break;
      case 'g':
      case 'G':
        window.open('https://github.com/adam-stokes/orcai', '_blank');
        break;
      case 'Escape':
        document.body.style.transition = 'opacity 0.4s';
        document.body.style.opacity    = '0';
        setTimeout(function() { document.body.style.opacity = '1'; }, 3000);
        break;
    }
  });
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

// ── Scroll reveal ─────────────────────────────────────────────
function initScrollReveal() {
  var cards = document.querySelectorAll('.feature-card, .plugin-tier');
  if (!cards.length || !('IntersectionObserver' in window)) return;

  var obs = new IntersectionObserver(function(entries) {
    entries.forEach(function(entry) {
      if (entry.isIntersecting) {
        entry.target.style.transition = 'opacity 0.5s, transform 0.5s';
        entry.target.style.opacity    = '1';
        entry.target.style.transform  = 'translateY(0)';
        obs.unobserve(entry.target);
      }
    });
  }, { threshold: 0.1 });

  cards.forEach(function(card) {
    card.style.opacity   = '0';
    card.style.transform = 'translateY(16px)';
    obs.observe(card);
  });
}

// ── Node status clock ─────────────────────────────────────────
function initNodeStatus() {
  var el = document.querySelector('.node-status .clock');
  if (!el) return;

  function tick() {
    el.textContent = new Date().toUTCString().replace('GMT', 'UTC');
  }
  tick();
  setInterval(tick, 1000);
}

// ── Boot ───────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', function() {
  initHexCanvas();
  colorLogo();

  var tw = document.getElementById('typewriter');
  if (tw) {
    typewriter(tw, [
      'AI in your terminal, not your browser',
      'tmux-native \u00b7 YAML pipelines \u00b7 any CLI as a plugin',
      'Claude \u00b7 Ollama \u00b7 Copilot \u00b7 local models',
      'Ctrl+; to open the chord menu',
    ]);
  }

  if (document.querySelector('[data-page="index"]')) {
    initKeyboardNav();
  }

  initCopyButtons();
  initScrollReveal();
  initNodeStatus();
});
