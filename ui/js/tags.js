// ---- Tag dialog: list, create (lightweight / annotated), push and delete ----
// S.tagTarget is the commit the next tag will point at ('' means HEAD).
// TG.remote is null while the remote is being checked, false when it could not be
// reached (TG.remoteErr says why), or a Set with the tag names that exist there.
const TG = { list: [], remote: null, remoteErr: '', seq: 0 };

// Open the dialog. With a commit hash (from the commit menu) that commit is the target;
// from the header button the selected commit, if any, is used.
async function openTags(target) {
  if (!S.cur) return;
  S.tagTarget = typeof target === 'string' ? target : S.sel || '';
  $('tname').value = '';
  $('tmsg').value = '';
  $('tmodal').style.display = 'grid';
  drawTagTarget();
  $('tname').focus();
  await loadTags();
}
function closeTags() {
  $('tmodal').style.display = 'none';
}
function setTagTarget(h) {
  S.tagTarget = h;
  drawTagTarget();
}
function drawTagTarget() {
  const head = S.commits[S.commits.length - 1];
  const c = S.tagTarget ? S.commits.find(x => x.Hash === S.tagTarget) : head;
  const hash = S.tagTarget || (head ? head.Hash : '');
  $('ttarget').innerHTML =
    (S.tagTarget ? '対象: ' : '対象: HEAD ') +
    (hash
      ? `<code>${esc(hash.slice(0, 8))}</code> ${c ? subj(c.Subject) : ''}`
      : 'コミットがありません') +
    (S.tagTarget ? ' <button onclick="setTagTarget(\'\')">HEADにする</button>' : '');
}
async function loadTags() {
  const repo = S.cur;
  const ts = await T(() => A('tags?repo=' + encodeURIComponent(repo)));
  if (!Array.isArray(ts) || S.cur !== repo) return;
  TG.list = ts;
  TG.remote = null;
  drawTagList();
  loadRemoteTags(repo);
}
// The remote check talks to the network, so it runs after the list is already on screen
async function loadRemoteTags(repo) {
  const seq = ++TG.seq;
  try {
    const names = await A('tagremote?repo=' + encodeURIComponent(repo));
    if (seq !== TG.seq) return;
    TG.remote = new Set(names);
  } catch (e) {
    if (seq !== TG.seq) return;
    TG.remote = false;
    TG.remoteErr = e.message;
  }
  drawTagList();
}
function remoteBadge(name) {
  if (!(TG.remote instanceof Set)) return '';
  return TG.remote.has(name)
    ? '<span class="tk ok">push済み</span>'
    : '<span class="tk warn">未push</span>';
}
function drawTagList() {
  const n = TG.list.length;
  const note =
    TG.remote === null ? ' / リモートを確認中...' : TG.remote === false ? ' / ' + TG.remoteErr : '';
  $('tcount').textContent = `${n} 件のタグ${note}`;
  $('tlist').innerHTML =
    TG.list
      .map(t => {
        const when = new Date(t.time * 1000).toLocaleDateString('ja-JP', {
          year: 'numeric',
          month: 'numeric',
          day: 'numeric',
        });
        return `<div class="te"><div class="tm"><div class="tn"><b>${esc(t.name)}</b>${
          t.annotated ? '<span class="tk">注釈付き</span>' : ''
        }${remoteBadge(t.name)}</div>${
          t.message ? `<div class="ts" title="${esc(t.message)}">${esc(t.message)}</div>` : ''
        }</div><time>${when}</time><code>${esc(t.hash.slice(0, 8))}</code><button data-n="${esc(
          t.name,
        )}" onclick="tagMenu(event,this.dataset.n)">操作</button></div>`;
      })
      .join('') || '<div class="empty">タグはまだありません</div>';
}
// Per-tag actions. stopPropagation keeps the global click handler from closing the menu at once
function tagMenu(e, name) {
  e.stopPropagation();
  const t = TG.list.find(x => x.name === name);
  if (!t) return;
  const onRemote = !(TG.remote instanceof Set) || TG.remote.has(name);
  const items = [];
  if (t.hash) items.push(['コミットへ移動', () => jumpTag(t.hash)]);
  items.push(
    ['リモートにpush', () => pushTag(name)],
    ['タグ名をコピー', () => copyText(name)],
    null,
  );
  items.push(['ローカルから削除', () => delTag(name, false), 'x']);
  if (onRemote) items.push(['リモートから削除', () => delTag(name, true), 'x']);
  menu(e, items);
}
// Close the dialog and select the tagged commit in the log
function jumpTag(h) {
  if (!S.commits.some(c => c.Hash === h)) return toast('履歴にそのコミットが見つかりません');
  closeTags();
  S.sel = h;
  S.tab = 0;
  drawLog();
  tabs();
  drawRight();
  const el = document.querySelector('#msgs .c.sel');
  if (el) el.scrollIntoView({ block: 'center' });
}
// Reload after any change; refresh() redraws the log so the badges follow
async function afterTagChange() {
  await loadTags();
  await refresh();
}
function createTag() {
  const name = $('tname').value.trim();
  if (!name) return toast('タグ名を入力してください');
  const repo = S.cur;
  T(async () => {
    $('tok').disabled = true;
    try {
      await A('tagadd', {
        repo,
        name,
        hash: S.tagTarget || 'HEAD',
        message: $('tmsg').value.trim(),
      });
      $('tname').value = '';
      $('tmsg').value = '';
      if (!$('tpush').checked) toast('タグを作成しました', true);
      else {
        // The tag already exists here, so a push failure must not look like a failed create
        try {
          await A('tagpush', { repo, name });
          toast('タグを作成してpushしました', true);
        } catch (e) {
          toast('タグは作成しましたが、pushに失敗しました:\n' + e.message);
        }
      }
    } finally {
      $('tok').disabled = false;
    }
    await afterTagChange();
  });
}
// name '' pushes every tag
function pushTag(name) {
  const repo = S.cur;
  T(async () => {
    toast(name ? `タグ「${name}」をpushしています` : 'すべてのタグをpushしています', true);
    await A('tagpush', { repo, name });
    toast('pushしました', true);
    loadRemoteTags(repo);
  });
}
// remote=false deletes the local tag only; remote=true deletes it on the remote only
function delTag(name, remote) {
  const msg = remote
    ? `リモートのタグ「${name}」を削除します。他の環境で取得済みの場合は、そちらには残ります。\nよろしいですか?`
    : `ローカルのタグ「${name}」を削除します。リモートにあるタグは残ります。\nよろしいですか?`;
  if (!confirm(msg)) return;
  const repo = S.cur;
  T(async () => {
    await A('tagdel', { repo, name, remote });
    toast('削除しました', true);
    await afterTagChange();
  });
}
$('tname').onkeydown = e => {
  if (e.key === 'Escape') closeTags();
  // Ignore the Enter that confirms an IME conversion (keyCode 229 covers older WebViews)
  if (e.key === 'Enter' && !e.isComposing && e.keyCode !== 229) {
    e.preventDefault();
    createTag();
  }
};
$('tmsg').onkeydown = e => {
  if (e.key === 'Escape') closeTags();
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !e.isComposing) createTag();
};
