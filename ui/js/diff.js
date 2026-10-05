S.split = localStorage.split === '1';
// Parse a unified diff into rows with old/new line numbers; header noise (diff/index/---/+++) is dropped
function parseDiff(t) {
  const rows = [],
    st = { a: 0, d: 0 };
  let o = 0,
    n = 0,
    hunk = false;
  for (const l of t.split('\n')) {
    if (l.startsWith('diff --git') || l.startsWith('diff --cc')) {
      hunk = false;
      continue;
    }
    const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@ ?(.*)$/.exec(l);
    if (m) {
      o = +m[1];
      n = +m[2];
      hunk = true;
      rows.push({ k: 'h', n: n, x: m[3] });
      continue;
    }
    if (!hunk) {
      if (/^(Binary files|rename |copy |new file|deleted file|old mode|new mode)/.test(l))
        rows.push({ k: 'm', x: l });
      continue;
    }
    const c = l[0],
      s = l.slice(1);
    if (c === '+') {
      rows.push({ k: 'a', n: n++, x: s });
      st.a++;
    } else if (c === '-') {
      rows.push({ k: 'd', o: o++, x: s });
      st.d++;
    } else if (c === ' ') rows.push({ k: 'c', o: o++, n: n++, x: s });
    else if (c === '\\') rows.push({ k: 'm', x: l.slice(2) });
  }
  return { rows, st };
}
// Intra-line highlight: mark only the differing middle part of a changed line pair
function hlPair(a, b) {
  const m = Math.min(a.length, b.length);
  let p = 0;
  while (p < m && a[p] === b[p]) p++;
  let s = 0;
  while (s < m - p && a[a.length - 1 - s] === b[b.length - 1 - s]) s++;
  // Skip marking when the lines are mostly different; whole-line color is clearer then
  if (p + s < Math.max(a.length, b.length) * 0.3) return [esc(a), esc(b)];
  const w = x => {
    const mid = x.slice(p, x.length - s);
    return (
      esc(x.slice(0, p)) + (mid ? '<mark>' + esc(mid) + '</mark>' : '') + esc(x.slice(x.length - s))
    );
  };
  return [w(a), w(b)];
}
// Pair each run of removed lines with the following run of added lines
function pairRows(rows) {
  for (let i = 0; i < rows.length;) {
    if (rows[i].k !== 'd') {
      i++;
      continue;
    }
    let j = i;
    while (j < rows.length && rows[j].k === 'd') j++;
    let e = j;
    while (e < rows.length && rows[e].k === 'a') e++;
    for (let k = 0; k < Math.min(j - i, e - j); k++) {
      const [x, y] = hlPair(rows[i + k].x, rows[j + k].x);
      rows[i + k].h = x;
      rows[j + k].h = y;
    }
    i = e;
  }
}
const cellH = r => (r.h !== undefined ? r.h : esc(r.x)) || ' ';
const hunkRow = r => `<div class="r h">${r.n}行目付近${r.x ? '  ' + esc(r.x) : ''}</div>`;
const noteRow = r => `<div class="r m">${esc(r.x)}</div>`;
function renderUnified(rows) {
  return (
    '<div class="db">' +
    rows
      .map(r =>
        r.k === 'h'
          ? hunkRow(r)
          : r.k === 'm'
            ? noteRow(r)
            : `<div class="r ${r.k}"><span class="g">${r.o ?? ''}</span><span class="g">${r.n ?? ''}</span><span class="s">${r.k === 'a' ? '+' : r.k === 'd' ? '-' : ' '}</span><span class="t">${cellH(r)}</span></div>`,
      )
      .join('') +
    '</div>'
  );
}
function renderSplit(rows) {
  const out = [];
  for (let i = 0; i < rows.length;) {
    const r = rows[i];
    if (r.k === 'd') {
      let j = i;
      while (j < rows.length && rows[j].k === 'd') j++;
      let e = j;
      while (e < rows.length && rows[e].k === 'a') e++;
      for (let k = 0; k < Math.max(j - i, e - j); k++)
        out.push({ l: k < j - i ? rows[i + k] : null, r: k < e - j ? rows[j + k] : null });
      i = e;
    } else if (r.k === 'a') {
      out.push({ l: null, r });
      i++;
    } else if (r.k === 'c') {
      out.push({ l: r, r });
      i++;
    } else {
      out.push({ full: r });
      i++;
    }
  }
  const side = (r, ln) =>
    r
      ? `<span class="g ${r.k}">${r[ln]}</span><span class="t ${r.k}">${cellH(r)}</span>`
      : '<span class="g e"></span><span class="t e"></span>';
  return (
    '<div class="db sbs">' +
    out
      .map(p =>
        p.full
          ? p.full.k === 'h'
            ? hunkRow(p.full)
            : noteRow(p.full)
          : `<div class="r">${side(p.l, 'o')}${side(p.r, 'n')}</div>`,
      )
      .join('') +
    '</div>'
  );
}
function showDw() {
  $('dwrap').style.display = 'flex';
  document.body.classList.add('dv');
}
// Closing the viewer brings the commit log back
function closeDiff() {
  $('dwrap').style.display = 'none';
  document.body.classList.remove('dv');
  $('diff').textContent = '';
}
function toggleMode() {
  S.split = !S.split;
  localStorage.split = S.split ? 1 : 0;
  if (S.dt) paint(S.dt.t, S.dt.n, true);
}
document.addEventListener('keydown', e => {
  if (e.key === 'Escape' && $('dwrap').style.display === 'flex') closeDiff();
});
function paint(t, n, keep) {
  S.dt = { t, n };
  const D = $('diff'),
    y = D.scrollTop;
  $('dname').textContent = n;
  showDw();
  if (!t.trim()) {
    D.className = '';
    D.innerHTML = '<div class="empty">差分はありません</div>';
    $('dstat').textContent = '';
    $('dmode').style.display = 'none';
    return;
  }
  const { rows, st } = parseDiff(t);
  // Not a diff (e.g. plain file content fallback): show as numbered text
  if (!rows.some(r => r.k === 'h' || r.k === 'm')) {
    D.className = 'file';
    $('dstat').textContent = '';
    $('dmode').style.display = 'none';
    D.innerHTML = renderPlain(t);
    D.scrollTop = 0;
    return;
  }
  pairRows(rows);
  D.className = 'dff';
  $('dstat').innerHTML = `<span class="a">+${st.a}</span> <span class="d">-${st.d}</span>`;
  $('dmode').style.display = '';
  $('dmode').textContent = S.split ? '統合表示' : '分割表示';
  D.innerHTML = S.split ? renderSplit(rows) : renderUnified(rows);
  D.scrollTop = keep ? y : 0;
}
