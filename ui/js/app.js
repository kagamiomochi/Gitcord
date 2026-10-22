// ---- Composer: prefix buttons and send controls ----
// Rebuild the prefix buttons from the current settings, keeping the selected one highlighted
function renderPf() {
  $('pf').innerHTML = PF.map(
    p =>
      `<button data-p="${esc(p)}" style="--pc:${esc(PFC[p])}" onclick="setPf(this.dataset.p)">${esc(p)}</button>`,
  ).join('');
  document
    .querySelectorAll('#pf button')
    .forEach(b => b.classList.toggle('on', b.dataset.p === S.pf));
}
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
// Dropdown action: overwrite the remote history after a rewrite (refuses if someone else pushed meanwhile)
function forcePush() {
  $('sm').style.display = 'none';
  if (!S.cur || S.busy) return;
  if (!confirm('force push (--force-with-lease) でリモートの履歴を上書きします。よろしいですか?'))
    return;
  T(async () => {
    S.busy = true;
    stat('force pushしています');
    try {
      await A('push', { repo: S.cur, force: true });
      toast('force pushしました', true);
    } finally {
      S.busy = false;
      stat('');
    }
    refresh();
  });
}
// Explain how to continue when a plain push was rejected, which is what happens after a rewrite
const pushErr = m =>
  /rejected|non-fast-forward|fetch first/.test(m)
    ? m + '\n\n履歴を書き換えた場合は、送信ボタン横の「▾」から force push できます'
    : m;
function toggleSm(e) {
  e.stopPropagation();
  if ($('sendm').disabled) return;
  const m = $('sm');
  m.style.display = m.style.display === 'block' ? 'none' : 'block';
}
$('msg').oninput = upd;
// Does this Enter keydown match the configured send key?
function isSendKey(e) {
  const mod = e.ctrlKey || e.metaKey;
  if (CFG.sendKey === 'enter') return !mod && !e.shiftKey && !e.altKey;
  if (CFG.sendKey === 'shift') return e.shiftKey && !mod && !e.altKey;
  return mod && !e.shiftKey && !e.altKey;
}
$('msg').onkeydown = e => {
  // Ignore the Enter that confirms an IME conversion (keyCode 229 covers older WebViews)
  if (e.key !== 'Enter' || e.isComposing || e.keyCode === 229 || !isSendKey(e)) return;
  e.preventDefault();
  // Explain why nothing happens instead of silently swallowing the key
  if (S.cur && !S.busy && $('msg').value.trim() && !S.pf) toast('プレフィックスを選んでください');
  act();
};
applyCfg();
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
  $('ht').textContent = p.split('/').pop();
  $('head').classList.add('on');
  loadRepos();
  await refresh(true);
}
async function refresh(bottom) {
  if (!S.cur) return;
  const l = await A('log?repo=' + encodeURIComponent(S.cur));
  S.commits = l.commits.reverse();
  S.up = new Set(l.unpushed);
  S.tags = l.tags || {};
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
          `<div class="c ${S.up.has(c.Hash) ? 'up' : ''} ${S.sel === c.Hash ? 'sel' : ''}" data-h="${c.Hash}" onclick="selC('${c.Hash}')" oncontextmenu="cMenu(event,'${c.Hash}')"><div class="av" style="${hue(c.Author)}">${esc(c.Author[0] || '?')}</div><div class="m"><b>${esc(c.Author)}</b><time>${new Date(c.Time * 1000).toLocaleTimeString('ja-JP', { hour: '2-digit', minute: '2-digit' })}</time>${S.up.has(c.Hash) ? '<span class="tag">未プッシュ</span>' : ''}${tagBadges(c.Hash)}<div>${subj(c.Subject)}</div><code>${c.Hash.slice(0, 8)}</code></div></div>`,
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
// Warning appended to confirmations when an action rewrites already pushed commits
const PUSHED_WARN =
  'このコミットはpush済みです。履歴を書き換えると、反映にforce pushが必要になります。\n他の環境で取得済みの場合は履歴が食い違います。';
// Rewriting history leaves tags on the old commits; name the ones on commit index i or later
function tagWarn(i) {
  if (i < 0) return '';
  const names = S.commits.slice(i).flatMap(c => S.tags[c.Hash] || []);
  return names.length
    ? `タグ(${names.join(', ')})は書き換え前のコミットに残り、新しい履歴からは外れます。`
    : '';
}
// Actions after which commit hashes change, so a selected commit may no longer exist
const REWRITES = new Set(['soft', 'mixed', 'hard', 'reword', 'drop', 'fixup', 'restore']);
function cMenu(e, h) {
  e.preventDefault();
  const i = S.commits.findIndex(c => c.Hash === h);
  if (i < 0) return;
  const pushed = !S.up.has(h);
  // The log is capped, so only a short history is known to start at the real first commit
  const isRoot = i === 0 && S.commits.length < 5000;
  const isHead = i === S.commits.length - 1;
  // from: first commit index whose hash changes, used to list the tags that would be left behind
  const ask = (msg, rewrites, from = i) =>
    [msg, ...(rewrites ? [pushed ? PUSHED_WARN : '', tagWarn(from)] : [])]
      .filter(Boolean)
      .join('\n\n');
  // Run a history action; msg (optional) asks for confirmation first
  const run = (a, msg) => () => {
    if (msg && !confirm(msg)) return;
    T(async () => {
      await A('action', { repo: S.cur, hash: h, action: a });
      if (REWRITES.has(a)) S.sel = null;
      await refresh();
      toast('完了', true);
    });
  };
  const items = [
    ['ハッシュをコピー', () => copyText(h)],
    [
      'メッセージをコピー',
      () => T(async () => copyText(await A(`message?repo=${encodeURIComponent(S.cur)}&rev=${h}`))),
    ],
    null,
    ['メッセージを編集', () => editMsg(h, pushed)],
    ['タグを作成...', () => openTags(h)],
    ...(S.tags[h] || []).map(n => [`タグ「${n}」を削除`, () => delTag(n, false), 'x']),
  ];
  if (!isRoot) {
    items.push(
      [
        '1つ前のコミットにまとめる (fixup)',
        run(
          'fixup',
          ask(
            'このコミットを1つ前のコミットにまとめます。メッセージは前のコミットのものが残ります。',
            true,
          ),
        ),
      ],
      [
        '履歴から削除 (drop)',
        run(
          'drop',
          ask('このコミットを履歴から削除し、以降のコミットを付け替えます。よろしいですか?', true),
        ),
        'x',
      ],
    );
  }
  items.push(
    null,
    ['打ち消しコミットを作成 (revert)', run('revert')],
    ['打ち消しをステージだけする (revert --no-commit)', run('revert-n')],
  );
  if (!isHead) {
    items.push(
      null,
      [
        'reset --soft (変更を残す・ステージ維持)',
        run('soft', ask('このコミットまで戻します。', true, i + 1)),
      ],
      ['reset --mixed (変更を残す)', run('mixed', ask('このコミットまで戻します。', true, i + 1))],
      [
        'reset --hard (変更を破棄)',
        run(
          'hard',
          ask('このコミットまで戻し、以降の変更を破棄します。よろしいですか?', true, i + 1),
        ),
        'x',
      ],
    );
  }
  menu(e, items);
}
// Items are [label, handler, className]; null draws a separator
function menu(e, items) {
  const m = $('menu');
  m.innerHTML = '';
  items.forEach(it => {
    const d = document.createElement('div');
    if (!it) {
      d.className = 'sep';
      m.append(d);
      return;
    }
    const [l, f, c] = it;
    d.textContent = l;
    if (c) d.className = c;
    d.onclick = () => {
      m.style.display = 'none';
      f();
    };
    m.append(d);
  });
  m.style.display = 'block';
  // Measure after showing so the menu never runs off the bottom of the window
  m.style.left = Math.max(0, Math.min(e.clientX, innerWidth - m.offsetWidth - 10)) + 'px';
  m.style.top = Math.max(0, Math.min(e.clientY, innerHeight - m.offsetHeight - 10)) + 'px';
}
async function copyText(t) {
  try {
    await navigator.clipboard.writeText(t);
  } catch {
    // Fallback for WebViews without the async clipboard API
    const a = document.createElement('textarea');
    a.value = t;
    document.body.append(a);
    a.select();
    document.execCommand('copy');
    a.remove();
  }
  toast('コピーしました', true);
}
// ---- Edit a commit message ----
async function editMsg(h, pushed) {
  const m = await T(() => A(`message?repo=${encodeURIComponent(S.cur)}&rev=${h}`));
  if (typeof m !== 'string') return;
  S.edit = { h, pushed };
  $('emsg').value = m;
  $('ehint').textContent = [
    pushed
      ? 'push済みのコミットです。保存すると履歴が書き換わり、反映にはforce pushが必要になります。'
      : '',
    tagWarn(S.commits.findIndex(c => c.Hash === h)),
  ]
    .filter(Boolean)
    .join('\n');
  $('emodal').style.display = 'grid';
  $('emsg').focus();
}
function closeEdit() {
  $('emodal').style.display = 'none';
  S.edit = null;
}
function saveEdit() {
  const ed = S.edit,
    message = $('emsg').value.trim();
  if (!ed) return;
  if (!message) return toast('メッセージを入力してください');
  T(async () => {
    $('eok').disabled = true;
    try {
      await A('action', { repo: S.cur, hash: ed.h, action: 'reword', message });
    } finally {
      $('eok').disabled = false;
    }
    closeEdit();
    S.sel = null;
    await refresh();
    toast('完了', true);
  });
}
$('emsg').onkeydown = e => {
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !e.isComposing) saveEdit();
  if (e.key === 'Escape') closeEdit();
};
// ---- Operation history (reflog): undo a mistaken rewrite ----
async function openReflog() {
  if (!S.cur) return;
  const repo = S.cur;
  const es = await T(() => A('reflog?repo=' + encodeURIComponent(repo)));
  if (!Array.isArray(es) || S.cur !== repo) return;
  // Intermediate steps of a rebase are not meaningful places to return to
  const keep = es.filter(
    (x, i) => i === 0 || !/^rebase \(/.test(x.message) || /^rebase \(finish\)/.test(x.message),
  );
  $('rlist').innerHTML =
    keep
      .map((x, i) => {
        const t = new Date(x.time * 1000).toLocaleString('ja-JP', {
          month: 'numeric',
          day: 'numeric',
          hour: '2-digit',
          minute: '2-digit',
        });
        return `<div class="re"><time>${t}</time><span class="rm" title="${esc(x.message)}">${esc(x.message)}</span><code>${x.hash.slice(0, 8)}</code>${
          i === 0
            ? '<span class="cur">現在</span>'
            : `<button data-h="${x.hash}" onclick="restoreTo(this.dataset.h)">戻す</button>`
        }</div>`;
      })
      .join('') || '<div class="empty">履歴がありません</div>';
  $('rmodal').style.display = 'grid';
}
function closeReflog() {
  $('rmodal').style.display = 'none';
}
function restoreTo(h) {
  const repo = S.cur;
  T(async () => {
    await A('action', { repo, hash: h, action: 'restore' });
    closeReflog();
    S.sel = null;
    await refresh();
    toast('元に戻しました', true);
  });
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
            return fileRow({
              path: f.path,
              onclick: `showDiff(${i})`,
              ondbl: `openExt(S._st[${i}].path)`,
              lead,
            });
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
              ondbl: `openExt(S._cs[${i}].path, S.sel)`,
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
          : `<div class="f t ${S.file === n[k] ? 'on' : ''}" data-p="${esc(n[k])}" onclick="openFile(this.dataset.p)" ondblclick="openExt(this.dataset.p, S.sel)"><span class="sp"></span><span style="color:${extColor(k)};display:flex">${ICO.file}</span><span class="nm">${esc(k)}</span></div>`,
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
// Open a file with the OS default application (double-click).
// rev empty: the working-tree file; otherwise a read-only copy of that commit's version
function openExt(p, rev) {
  return T(async () => {
    await A('open', { repo: S.cur, path: p, rev: rev || '' });
    toast('開きました: ' + splitPath(p).name, true);
  });
}
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
          m = await A('translate', trBody(m));
        } catch (e) {
          if (!confirm('英訳に失敗しました: ' + e.message + '\n原文のままコミットしますか?'))
            return;
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
          toast('コミットは完了しましたが、pushに失敗しました:\n' + pushErr(e.message));
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
      try {
        await A('push', { repo: S.cur });
      } catch (e) {
        throw new Error(pushErr(e.message));
      }
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
        await A('clone', {
          url: $('curl').value.trim(),
          dir: S.dir,
          name: $('cname').value.trim(),
        });
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
