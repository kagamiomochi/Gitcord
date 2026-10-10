// ---- Settings dialog: send key and commit prefixes ----
// Every change is saved to localStorage immediately and applied live.

// Apply CFG to the composer: rebuild prefix buttons, placeholder, and redraw log badges
function applyCfg() {
  rebuildPf();
  // The selected prefix may have been renamed or deleted
  if (S.pf && !PF.includes(S.pf)) S.pf = null;
  renderPf();
  $('msg').placeholder = `コミットメッセージ (${SEND_HINT[CFG.sendKey]})`;
  upd();
  if (S.cur) drawLog();
}
function openSettings() {
  document.querySelectorAll('input[name=sk]').forEach(r => (r.checked = r.value === CFG.sendKey));
  drawPfList();
  $('smodal').style.display = 'grid';
}
function closeSettings() {
  $('smodal').style.display = 'none';
}
function setSendKey(k) {
  if (!(k in SEND_HINT)) return;
  CFG.sendKey = k;
  saveCfg();
  applyCfg();
}
// Returns an error message for an invalid prefix name, or '' if it is usable (skip = index being edited)
function pfErr(name, skip) {
  if (!name) return 'プレフィックス名を入力してください';
  if (!PF_NAME_RE.test(name)) return '空白・コロン・括弧・!は使えません';
  if (name === 'none') return '"none" は予約されています';
  if (CFG.prefixes.some((p, i) => i !== skip && p.name === name)) return '同じ名前が既に存在します';
  return '';
}
function drawPfList() {
  const last = CFG.prefixes.length - 1;
  $('pfl').innerHTML =
    CFG.prefixes
      .map(
        (p, i) =>
          `<div class="pr"><input type="color" value="${esc(p.color)}" onchange="editPf(${i},'color',this.value)"><input class="pn" value="${esc(p.name)}" maxlength="20" onchange="editPf(${i},'name',this.value)"><button ${i === 0 ? 'disabled' : ''} onclick="movePf(${i},-1)" title="上へ">&uarr;</button><button ${i === last ? 'disabled' : ''} onclick="movePf(${i},1)" title="下へ">&darr;</button><button class="x" onclick="delPf(${i})">削除</button></div>`,
      )
      .join('') +
    `<div class="pr fixed"><span class="sq" style="background:${NONE_COLOR}"></span><span class="pn">none</span><span class="mu">プレフィックスなし (常に表示されます)</span></div>`;
}
function changedPf() {
  saveCfg();
  applyCfg();
  drawPfList();
}
function editPf(i, key, v) {
  if (key === 'name') {
    v = v.trim();
    const e = pfErr(v, i);
    if (e) {
      toast(e);
      drawPfList();
      return;
    }
  }
  CFG.prefixes[i][key] = v;
  changedPf();
}
function movePf(i, d) {
  const j = i + d;
  if (j < 0 || j >= CFG.prefixes.length) return;
  [CFG.prefixes[i], CFG.prefixes[j]] = [CFG.prefixes[j], CFG.prefixes[i]];
  changedPf();
}
function delPf(i) {
  CFG.prefixes.splice(i, 1);
  changedPf();
}
function addPf() {
  const name = $('pfn').value.trim();
  const e = pfErr(name, -1);
  if (e) {
    toast(e);
    return;
  }
  CFG.prefixes.push({ name, color: $('pfc').value });
  $('pfn').value = '';
  changedPf();
}
function resetPf() {
  if (!confirm('プレフィックスを初期設定に戻します。よろしいですか?')) return;
  CFG.prefixes = DEF_PF.map(p => ({ ...p }));
  changedPf();
}
$('smodal').onclick = e => {
  if (e.target === e.currentTarget) closeSettings();
};
$('pfn').onkeydown = e => {
  if (e.key === 'Enter' && !e.isComposing && e.keyCode !== 229) addPf();
};
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') closeSettings();
});
