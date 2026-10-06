const $ = id => document.getElementById(id),
  esc = s =>
    String(s).replace(
      /[&<>"]/g,
      c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c],
    );
const ICO = {
  dir: '<svg class="ic" viewBox="0 0 16 16"><path d="M1.5 3.5a1 1 0 0 1 1-1h3.2l1.4 1.5h6.4a1 1 0 0 1 1 1v7.5a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1z" fill="currentColor"/></svg>',
  file: '<svg class="ic" viewBox="0 0 16 16"><path d="M3.5 1.5h5.6l3.4 3.4v9a1 1 0 0 1-1 1h-8a1 1 0 0 1-1-1v-11.4a1 1 0 0 1 1-1z" fill="none" stroke="currentColor" stroke-width="1.2"/><path d="M9 1.7V5h3.3" fill="none" stroke="currentColor" stroke-width="1.2"/></svg>',
  chev: '<svg class="chev" viewBox="0 0 16 16"><path d="M6 3.5l4.5 4.5L6 12.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>',
};
const EC = {
  js: '#e5c34b',
  mjs: '#e5c34b',
  ts: '#4a9eff',
  tsx: '#4a9eff',
  html: '#e8764b',
  css: '#5aa9e6',
  json: '#c9a24b',
  md: '#8fa3b8',
  go: '#4fc3d9',
  mod: '#4fc3d9',
  rs: '#e0875a',
  toml: '#b0b0b0',
  yml: '#d27a7a',
  yaml: '#d27a7a',
  sh: '#6bcb77',
  py: '#5b9bd5',
  png: '#b57edc',
  svg: '#b57edc',
  jpg: '#b57edc',
};
const extColor = n => EC[(n.split('.').pop() || '').toLowerCase()] || 'var(--mu)';
const PF = ['feat', 'fix', 'docs', 'style', 'refactor', 'perf', 'test', 'chore', 'none'];
let S = {
  busy: false,
  repos: [],
  cur: null,
  commits: [],
  up: new Set(),
  sel: null,
  pf: null,
  tab: 0,
  mode: 0,
  dir: '',
};
$('tr').checked = localStorage.tr !== '0';
async function A(p, b) {
  const r = await fetch(
    '/api/' + p,
    b
      ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(b) }
      : {},
  );
  const j = await r.json();
  if (!r.ok) throw new Error(j.error);
  return j;
}
// Show a toast; ok=true uses the success color instead of the error color
function toast(m, ok = false) {
  const t = $('toast');
  t.textContent = m;
  t.classList.toggle('ok', ok);
  t.style.display = 'block';
  clearTimeout(t._t);
  t._t = setTimeout(() => (t.style.display = 'none'), 5000);
}
const T = f => f().catch(e => toast(e.message));

// Split "a/b/c.txt" into its file name and directory
function splitPath(p) {
  const k = p.lastIndexOf('/');
  return { name: p.slice(k + 1), dir: k >= 0 ? p.slice(0, k) : '' };
}
// Plain text as one <div> per line (line numbers come from CSS counters)
function renderPlain(t) {
  return esc(t.replace(/\n$/, ''))
    .split('\n')
    .map(l => `<div>${l || ' '}</div>`)
    .join('');
}
// One row of a changed-file list: leading (checkbox/chip) + name + directory
function fileRow({ path, title, onclick, lead }) {
  const { name, dir } = splitPath(path);
  const t = title ? ` title="${esc(title)}"` : '';
  return `<div class="f cf"${t} onclick="${onclick}">${lead}<span class="cl"><span class="nm">${esc(name)}</span><span class="dir">${esc(dir)}</span></span></div>`;
}

// Stable avatar background derived from a string (same name always gets the same color)
const hue = s => {
  let h = 0;
  for (const c of String(s)) h = (h * 31 + c.charCodeAt(0)) % 360;
  return `background:hsl(${h} 50% 42%)`;
};

// Color per commit-message prefix (used for prefix buttons and the badge in the log)
const PFC = {
  feat: '#23a559',
  fix: '#f23f43',
  docs: '#3b9cff',
  style: '#c77dff',
  refactor: '#f0b232',
  perf: '#ff8a3d',
  test: '#2ec4b6',
  chore: '#8e949d',
  none: '#6b7078',
};
const PFRE = new RegExp(`^(${Object.keys(PFC).filter(k => k !== 'none').join('|')})(\\([^)]*\\))?!?:\\s*([\\s\\S]*)$`);
// Render a commit subject with its prefix as a colored badge
function subj(t) {
  const m = PFRE.exec(t);
  if (!m) return esc(t);
  return `<span class="pfx" style="--pc:${PFC[m[1]]}">${m[1]}${esc(m[2] || '')}</span>${esc(m[3])}`;
}
// Date divider shown when a commit falls on a different local day than the previous one
function dayDiv(c, prev) {
  const d = new Date(c.Time * 1000);
  if (prev && new Date(prev.Time * 1000).toDateString() === d.toDateString()) return '';
  const label = d.toLocaleDateString('ja-JP', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    weekday: 'short',
  });
  return `<div class="dd"><span>${label}</span></div>`;
}
