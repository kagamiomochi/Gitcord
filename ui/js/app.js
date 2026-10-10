$('pf').innerHTML = PF.map(p => `<button data-p="${p}" style="--pc:${PFC[p]}" onclick="setPf('${p}')">${p}</button>`).join(
  '',
);
// ---- Composer: prefix buttons and send controls ----
function setPf(p) {
  S.pf = p;
  document.querySelectorAll('#pf button').forEach(b => b.classList.toggle('on', b.dataset.p === p));
  upd();
}
// The send button always commits then pushes; commit-only and push-only live in the dropdown
const canCommit = () => !!(S.cur && S.pf && $('msg').value.trim());
function upd() {
  const c = canCommit();
  $('send').disabled = S.busy || !c;
  // Push-only does not need a commit message, so it only requires a selected repo
  $('sendm').disabled = S.busy || !S.cur;
  $('smc').classList.toggle('off', !c);
  if ($('sendm').disabled) $('sm').style.display = 'none';
}
function act() {
  if ($('send').disabled) return;
  doCommit(true);
}
// Dropdown action: commit only, without pushing
function commitOnly() {
  $('sm').style.display = 'none';
  if (!canCommit() || S.busy) return;
  doCommit(false);
}
// Dropdown action: push only, without committing
function pushOnly() {
  $('sm').style.display = 'none';
  if (!S.cur || S.busy) return;
  doPush();
}
function toggleSm(e) {
  e.stopPropagation();
  if ($('sendm').disabled) return;
  const m = $('sm');
  m.style.display = m.style.display === 'block' ? 'none' : 'block';
}
$('msg').oninput = upd;
$('msg').onkeydown = e => {
  if (e.key === 'Enter' && e.ctrlKey) act();
};
// ---- Repository list and commit log ----
async function loadRepos() {
  S.repos = await A('repos');
  $('repos').innerHTML =
    S.repos
      .map(
        p =>
          `<div class="repo ${p === S.cur ? 'on' : ''}" data-p="${esc(p)}" oncontextmenu="repoMenu(event,this.dataset.p)" onclick="pick(this.dataset.p)"><div class="av" style="${hue(p.split('/').pop())}">${esc(p.split('/').pop()[0] || '?').toUpperCase()}</div><div><b>${esc(p.split('/').pop())}</b><small>${esc(p)}</small></div></div>`,
      )
      .join('') || '<div class="empty">＋で追加</div>';
}
function repoMenu(e, p) {
  e.preventDefault();
  menu(e, [
    [
      '一覧から削除',
      () =>
        T(async () => {
          await A('remove', { path: p });
          if (S.cur === p) S.cur = null;
          loadRepos();
        }),
      'x',
    ],
  ]);
}
async function pick(p) {
  S.cur = p;
  S.sel = null;
  $('head').textContent = p.split('/').pop();
  $('head').classList.add('on');
  loadRepos();
  await refresh(true);
}
async function refresh(bottom) {
  if (!S.cur) return;
  const l = await A('log?repo=' + encodeURIComponent(S.cur));
  S.commits = l.commits.reverse();
  S.up = new Set(l.unpushed);
  upd();
  drawLog(bottom);
  drawRight();
}
function drawLog(bottom) {
  const m = $('msgs');
  const keep = m.scrollHeight - m.scrollTop;
  m.innerHTML =
    (S.commits.length
      ? '<div class="first">first commit</div>'
      : '<div class="empty">コミットはまだありません</div>') +
    S.commits
      .map(
        (c, i) =>
          dayDiv(c, S.commits[i - 1]) +
          `<div class="c ${S.up.has(c.Hash) ? 'up' : ''} ${S.sel === c.Hash ? 'sel' : ''}" data-h="${c.Hash}" onclick="selC('${c.Hash}')" oncontextmenu="cMenu(event,'${c.Hash}')"><div class="av" style="${hue(c.Author)}">${esc(c.Author[0] || '?')}</div><div class="m"><b>${esc(c.Author)}</b><time>${new Date(c.Time * 1000).toLocaleTimeString('ja-JP', { hour: '2-digit', minute: '2-digit' })}</time>${S.up.has(c.Hash) ? '<span class="tag">未プッシュ</span>' : ''}<div>${subj(c.Subject)}</div><code>${c.Hash.slice(0, 8)}</code></div></div>`,
      )
      .join('');
  m.scrollTop = bottom ? m.scrollHeight : m.scrollHeight - keep;
}
function selC(h) {
  S.sel = S.sel === h ? null : h;
  if (S.sel) S.tab = 0;
  drawLog();
  tabs();
  drawRight();
}
function cMenu(e, h) {
  e.preventDefault();
  const run = (a, msg) => () => {
    if (msg && !confirm(msg)) return;
    T(async () => {
      await A('action', { repo: S.cur, hash: h, action: a });
      await refresh();
      toast('完了', true);
    });
  };
  menu(
    e,
    S.up.has(h)
      ? [
          ['reset --soft (変更を残す・ステージ維持)', run('soft')],
          ['reset --mixed (変更を残す)', run('mixed')],
          [
            'reset --hard (変更を破棄)',
            run('hard', 'このコミットまで戻し、以降の変更を破棄します。よろしいですか?'),
            'x',
          ],
        ]
      : [['revert (打ち消しコミットを作成)', run('revert')]],
  );
}
function menu(e, items) {
  const m = $('menu');
  m.innerHTML = '';
  items.forEach(([l, f, c]) => {
    const d = document.createElement('div');
    d.textContent = l;
    if (c) d.className = c;
    d.onclick = () => {
      m.style.display = 'none';
      f();
    };
    m.append(d);
  });
  m.style.left = Math.min(e.clientX, innerWidth - 210) + 'px';
  m.style.top = Math.min(e.clientY, innerHeight - items.length * 34 - 10) + 'px';
  m.style.display = 'block';
}
document.addEventListener('click', () => {
  $('menu').style.display = 'none';
  $('sm').style.display = 'none';
});
// ---- Right pane: changes / files tabs ----
function tab(n) {
  S.tab = n;
  tabs();
  drawRight();
}
function tabs() {
  $('t0').classList.toggle('on', S.tab === 0);
  $('t1').classList.toggle('on', S.tab === 1);
}
async function drawRight() {
  const L = $('list');
  closeDiff();
  if (!S.cur) {
    L.innerHTML = '';
    return;
  }
  if (S.tab === 0 && S.sel) {
    await drawCommitChanges();
    return;
  }
  if (S.tab === 0) {
    const st = await A('status?repo=' + encodeURIComponent(S.cur));
    L.innerHTML = st.length
      ? stageBar(st) +
        st
          .map((f, i) => {
            const c = f.y !== ' ' ? f.y : f.x;
            const l = c === '?' ? 'U' : c;
            const lead =
              `<input type="checkbox" ${f.staged ? 'checked' : ''} onclick="event.stopPropagation()" onchange="stage(${i},this.checked)">` +
              `<span class="chip c${l}" title="${esc(f.x + f.y)}">${l}</span>`;
            return fileRow({ path: f.path, onclick: `showDiff(${i})`, lead });
          })
          .join('')
      : '<div class="empty">未コミットの変更はありません</div>';
    S._st = st;
  } else {
    S._paths = (
      await A('tree?repo=' + encodeURIComponent(S.cur) + (S.sel ? '&rev=' + S.sel : ''))
    ).filter(Boolean);
    L.innerHTML = `<div class="tb"><input id="ff" placeholder="ファイルを絞り込み" value="${esc(S.q || '')}" oninput="S.q=this.value;drawTree()"><button onclick="fold(1)">すべて展開</button><button onclick="fold(0)">折りたたみ</button></div><div class="rev"><span>${S.sel ? S.sel.slice(0, 8) + '' : 'HEAD'}</span><span>${S._paths.length} ファイル</span></div><div id="tree"></div>`;
    drawTree();
  }
}
// Changes tab for a selected past commit: list the files it touched; clicking one shows that commit's diff
async function drawCommitChanges() {
  const L = $('list'),
    h = S.sel,
    repo = S.cur;
  const cs = await T(() => A(`changes?repo=${encodeURIComponent(repo)}&rev=${h}`));
  // Ignore the response if the selection or tab changed while it was loading
  if (!Array.isArray(cs) || S.sel !== h || S.cur !== repo || S.tab !== 0) return;
  S._cs = cs;
  const c = S.commits.find(x => x.Hash === h);
  L.innerHTML =
    `<div class="rev"><span class="nm">${h.slice(0, 8)} ${esc(c ? c.Subject : '')}</span><span>${cs.length} ファイル</span></div><div class="rev"><button onclick="selC(S.sel)">未コミットの変更に戻る</button></div>` +
    (cs.length
      ? cs
          .map((f, i) =>
            fileRow({
              path: f.path,
              title: f.old ? f.old + ' → ' + f.path : f.path,
              onclick: `showCDiff(${i})`,
              lead: `<span class="chip c${esc(f.status)}">${esc(f.status)}</span>`,
            }),
          )
          .join('')
      : '<div class="empty">このコミットで変更されたファイルはありません</div>');
}
async function showCDiff(i) {
  const f = S._cs[i],
    h = S.sel;
  const t = await T(() =>
    A(
      `diff?repo=${encodeURIComponent(S.cur)}&rev=${h}&path=${encodeURIComponent(f.path)}` +
        (f.old ? `&old=${encodeURIComponent(f.old)}` : ''),
    ),
  );
  if (typeof t !== 'string') return;
  paint(t, f.path + '  @ ' + h.slice(0, 8));
}
function fold(o) {
  document.querySelectorAll('#tree details').forEach(d => (d.open = !!o));
}
function drawTree() {
  const q = (S.q || '').toLowerCase(),
    root = {};
  (q ? S._paths.filter(p => p.toLowerCase().includes(q)) : S._paths).forEach(p => {
    let n = root;
    p.split('/').forEach((s, i, a) => {
      n = n[s] ??= i === a.length - 1 ? p : {};
    });
  });
  const cnt = n => Object.values(n).reduce((a, v) => a + (typeof v === 'object' ? cnt(v) : 1), 0);
  const R = (n, d) =>
    Object.keys(n)
      .sort((a, b) => (typeof n[b] === 'object') - (typeof n[a] === 'object') || a.localeCompare(b))
      .map(k =>
        typeof n[k] === 'object'
          ? `<details ${d === 0 || q ? 'open' : ''}><summary>${ICO.chev}${ICO.dir}<span class="nm">${esc(k)}</span><span class="cn">${cnt(n[k])}</span></summary><div class="kids">${R(n[k], d + 1)}</div></details>`
          : `<div class="f t ${S.file === n[k] ? 'on' : ''}" data-p="${esc(n[k])}" onclick="openFile(this.dataset.p)"><span class="sp"></span><span style="color:${extColor(k)};display:flex">${ICO.file}</span><span class="nm">${esc(k)}</span></div>`,
      )
      .join('');
  $('tree').innerHTML =
    R(root, 0) ||
    '<div class="empty">' + (q ? '一致するファイルがありません' : 'ファイルなし') + '</div>';
}
// Toolbar above the changed-file list: stage or unstage every file at once
function stageBar(st) {
  const n = st.filter(f => f.staged).length;
  const all = n === st.length;
  return `<div class="rev"><span>${n} / ${st.length} ステージ済み</span><button onclick="stageAll(${!all})">${all ? 'すべて解除' : 'すべてステージ'}</button></div>`;
}
async function stageAll(on) {
  await T(async () => {
    await A('stage', { repo: S.cur, paths: S._st.map(f => f.path), stage: on });
    drawRight();
  });
}
async function stage(i, on) {
  await T(async () => {
    await A('stage', { repo: S.cur, paths: [S._st[i].path], stage: on });
    drawRight();
  });
}
async function showDiff(i) {
  const f = S._st[i];
  paint(
    await A(
      `diff?repo=${encodeURIComponent(S.cur)}&path=${encodeURIComponent(f.path)}&staged=${f.staged && f.y === ' ' ? 1 : 0}`,
    ),
    f.path,
  );
}
// Open a file from the file browser: show its content (at the selected commit, or HEAD) without diff coloring
async function openFile(p) {
  S.file = p;
  document.querySelectorAll('#tree .f').forEach(e => e.classList.toggle('on', e.dataset.p === p));
  const t = await T(() =>
    A(
      `file?repo=${encodeURIComponent(S.cur)}&path=${encodeURIComponent(p)}&rev=${S.sel || 'HEAD'}`,
    ),
  );
  if (typeof t !== 'string') return;
  $('diff').className = 'file';
  $('dname').textContent = p + (S.sel ? '  @ ' + S.sel.slice(0, 8) : '');
  $('dstat').textContent = '';
  $('dmode').style.display = 'none';
  S.dt = null;
  showDw();
  $('diff').scrollTop = 0;
  $('diff').innerHTML = renderPlain(t);
}
// ---- Commit and push ----
// push=true: commit then push (default), push=false: commit only
async function doCommit(push) {
  T(async () => {
    // Pin the target repo so switching repos mid-push cannot push the wrong one
    const repo = S.cur;
    S.busy = true;
    stat('準備中');
    try {
      let m = $('msg').value.trim();
      if ($('tr').checked && /[^\x00-\x7f]/.test(m)) {
        stat('英訳中');
        try {
          m = await A('translate', { text: m });
        } catch (e) {
          if (!confirm('英訳に失敗しました: ' + e.message + '\n原文のままコミットしますか?')) return;
        }
      }
      if (S.pf !== 'none') m = S.pf + ': ' + m;
      stat('コミット中');
      await A('commit', { repo, message: m });
      $('msg').value = '';
      S.pf = null;
      setPf(null);
      if (push) {
        await refresh(true);
        stat('pushしています');
        // The commit already succeeded here, so a push failure must not look like a failed commit
        try {
          await A('push', { repo });
          toast('送信しました', true);
        } catch (e) {
          toast('コミットは完了しましたが、pushに失敗しました:\n' + e.message);
        }
      }
    } finally {
      S.busy = false;
      stat('');
    }
    await refresh(true);
  });
}
function doPush() {
  if (!S.cur) return;
  T(async () => {
    S.busy = true;
    stat('pushしています');
    try {
      await A('push', { repo: S.cur });
      toast('pushしました', true);
    } finally {
      S.busy = false;
      stat('');
    }
    refresh();
  });
}
// Progress feedback while sending: spinner on the send button, status text, and a progress bar ('' clears it)
function stat(t) {
  $('comp').classList.toggle('busy', !!t);
  $('send').classList.toggle('spin', !!t);
  $('stat').textContent = t;
  upd();
}
// ---- Add-repository dialog ----
// add dialog
function openAdd() {
  $('modal').style.display = 'grid';
  mtab(0);
  browse('');
}
function closeM() {
  $('modal').style.display = 'none';
}
function mtab(n) {
  S.mode = n;
  $('clone').style.display = n ? 'block' : 'none';
  $('m0').className = n ? '' : 'pri';
  $('m1').className = n ? 'pri' : '';
  $('ok').textContent = n ? 'ここにclone' : 'このディレクトリを追加';
}
async function browse(p) {
  T(async () => {
    const r = await A('ls?path=' + encodeURIComponent(p));
    S.dir = r.path;
    $('path').value = r.path;
    $('fb').innerHTML =
      `<div class="f" data-p="${esc(r.parent)}" onclick="browse(this.dataset.p)">..</div>` +
      r.entries
        .map(
          e =>
            `<div class="f" data-p="${esc(r.path.replace(/\/$/, '') + '/' + e.name)}" onclick="browse(this.dataset.p)">${ICO.dir}<span class="nm">${esc(e.name)}</span>${e.git ? '<span class="badge" style="margin-left:auto">Git</span>' : ''}</div>`,
        )
        .join('');
    $('ok').dataset.git = r.git ? 1 : '';
  });
}
$('curl').oninput = () => {
  if (!$('cname').dataset.u)
    $('cname').value = $('curl')
      .value.replace(/\/+$/, '')
      .split(/[\/:]/)
      .pop()
      .replace(/\.git$/, '');
};
$('cname').oninput = () => ($('cname').dataset.u = 1);
function confirmAdd() {
  T(async () => {
    const ok = $('ok');
    ok.disabled = true;
    ok.classList.add('spin');
    try {
      if (S.mode) {
        await A('clone', { url: $('curl').value.trim(), dir: S.dir, name: $('cname').value.trim() });
      } else await A('add', { path: S.dir });
    } finally {
      ok.disabled = false;
      ok.classList.remove('spin');
    }
    closeM();
    await loadRepos();
  });
}
window.addEventListener('focus', () => {
  if (S.cur)
    T(async () => {
      upd();
      if (S.tab === 0 && !S.sel) drawRight();
    });
});
// Keep --comp-h in sync with the overlaid composer height so the last
// commit is never hidden behind it; stay pinned to the bottom if we were there
(function trackComposerHeight() {
  const comp = $('comp');
  const mid = $('mid');
  const msgs = $('msgs');
  new ResizeObserver(() => {
    const atBottom = msgs.scrollHeight - msgs.scrollTop - msgs.clientHeight < 4;
    mid.style.setProperty('--comp-h', comp.offsetHeight + 'px');
    if (atBottom) msgs.scrollTop = msgs.scrollHeight;
  }).observe(comp);
})();
loadRepos();
