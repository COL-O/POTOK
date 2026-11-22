'use strict';
/* ЯНТАРНАЯ СФЕРА — клиент комнаты. Весь пользовательский текст — только через textContent. */

const el = (id) => document.getElementById(id);
const setText = (id, v) => { const n = el(id); if (n) n.textContent = String(v); };
const SVG_NS = 'http://www.w3.org/2000/svg';
const svgEl = (tag, attrs) => { const n = document.createElementNS(SVG_NS, tag); for (const k in attrs) n.setAttribute(k, attrs[k]); return n; };

/* ---------- вкладки ---------- */
function go(name) {
  document.querySelectorAll('#tabs .tab').forEach((b) => b.classList.toggle('is-active', b.dataset.tab === name));
  document.querySelectorAll('main > .panel').forEach((p) => p.classList.toggle('is-active', p.id === 'panel-' + name));
  window.scrollTo({ top: 0, behavior: 'smooth' });
}
document.querySelectorAll('#tabs .tab').forEach((b) => b.addEventListener('click', () => go(b.dataset.tab)));
document.querySelectorAll('[data-goto]').forEach((b) => b.addEventListener('click', () => go(b.dataset.goto)));

/* ---------- чат ---------- */
const chatLog = el('chatLog');
const chatStatus = el('chatStatus');
const onlineDot = el('onlineDot');
const nameInput = el('chatName');
const textInput = el('chatText');
const sendBtn = el('chatSend');
let myName = '';
try { myName = localStorage.getItem('amber-name') || ''; } catch (_) { myName = ''; }
nameInput.value = myName;

const seen = new Set();
const fmtTime = (ts) => {
  const d = new Date((ts || 0) * 1000);
  return d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) + ' · ' +
         d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit' });
};

function addMessage(m) {
  if (!m || !m.id) return;
  if (seen.has(m.id)) return;
  seen.add(m.id);
  const div = document.createElement('div');
  if (m.system) {
    div.className = 'msg system';
    const body = document.createElement('div');
    body.className = 'body';
    body.textContent = m.text;
    div.appendChild(body);
  } else {
    div.className = 'msg' + (m.name === myName ? ' mine' : '');
    const who = document.createElement('div'); who.className = 'who';
    const whoName = document.createElement('span'); whoName.textContent = m.name || 'Аноним';
    const when = document.createElement('span'); when.className = 'when'; when.textContent = fmtTime(m.ts);
    who.append(whoName, when);
    const body = document.createElement('div'); body.className = 'body'; body.textContent = m.text;
    div.append(who, body);
  }
  chatLog.appendChild(div);
  chatLog.scrollTop = chatLog.scrollHeight;
}

function systemNote(text) {
  addMessage({ id: 'sys-' + Date.now(), system: true, text, ts: Math.floor(Date.now() / 1000) });
}

async function loadMessages() {
  try {
    const r = await fetch('/api/messages');
    const j = await r.json();
    chatLog.innerHTML = '';
    (j.messages || []).forEach(addMessage);
    if (!(j.messages || []).length) systemNote('Пока тихо. Скажите первое слово круга.');
  } catch (_) {
    chatLog.innerHTML = '';
    systemNote('Чат недоступен: сервер комнаты не отвечает.');
  }
}

function connectStream() {
  const es = new EventSource('/api/stream');
  es.onopen = () => { setText('chatStatus', 'соединение живое'); onlineDot.classList.add('live'); };
  es.onmessage = (e) => { try { addMessage(JSON.parse(e.data)); } catch (_) { /* пропускаем битую строку */ } };
  es.onerror = () => {
    es.close();
    onlineDot.classList.remove('live');
    setText('chatStatus', 'связь прервалась — переподключаюсь…');
    loadMessages().then(connectStream);
  };
}

loadMessages().then(connectStream);

el('chatForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  const name = nameInput.value.trim();
  const text = textInput.value.trim();
  if (!text) return;
  myName = name;
  try { localStorage.setItem('amber-name', name); } catch (_) { /* приватный режим */ }
  sendBtn.disabled = true;
  try {
    const r = await fetch('/api/messages', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, text }),
    });
    if (r.ok) { textInput.value = ''; setText('chatStatus', 'отправлено'); }
    else {
      const j = await r.json().catch(() => ({}));
      setText('chatStatus', 'ошибка: ' + (j.error || r.status));
    }
  } catch (err) {
    setText('chatStatus', 'сеть недоступна: ' + err);
  } finally {
    sendBtn.disabled = false;
    textInput.focus();
  }
});

/* ---------- состояние / дашборды / отделы ---------- */
const LAYER_LABEL = { front: 'front', polygon: 'polygon', growth: 'growth', stratos: 'stratos' };
const LAYER_NOTE = { front: 'наносекунды–мс', polygon: 'минуты–часы', growth: 'часы–дни', stratos: 'годы–века' };

function renderRegistry(reg) {
  if (!reg || !reg.available) { setText('regActive', '—'); setText('regSource', 'ошибка чтения реестра: ' + ((reg && reg.error) || 'неизвестно')); return; }
  setText('regActive', reg.active);
  setText('regLegacy', reg.legacy);
  setText('regTotal', reg.total);
  setText('regSource', 'Источник: ' + reg.source);
  const bars = el('layerBars');
  bars.innerHTML = '';
  const layers = reg.layers || {};
  const max = Math.max(1, ...Object.values(layers));
  ['front', 'polygon', 'growth', 'stratos'].forEach((key) => {
    const value = layers[key] || 0;
    const row = document.createElement('div'); row.className = 'bar-row';
    const label = document.createElement('div');
    label.innerHTML = ''; label.appendChild(document.createTextNode(LAYER_LABEL[key] + ' '));
    const small = document.createElement('span'); small.className = 'muted small'; small.textContent = LAYER_NOTE[key];
    label.appendChild(small);
    const bar = document.createElement('div'); bar.className = 'bar';
    const fill = document.createElement('i'); fill.style.width = (100 * value / max) + '%';
    bar.appendChild(fill);
    const num = document.createElement('div'); num.textContent = value;
    row.append(label, bar, num);
    bars.appendChild(row);
  });
  setText('layerSource', 'Активных: ' + reg.active + ' · источник: registry/models.yaml');
}

function renderMemory(mem) {
  if (!mem || !mem.available) {
    setText('memNotes', '—'); setText('memJournal', '—'); setText('memRuns', '—');
    setText('memSource', 'память недоступна: ' + ((mem && mem.error) || 'неизвестно'));
    return;
  }
  setText('memNotes', mem.notes); setText('memJournal', mem.journal); setText('memRuns', mem.runs);
  setText('memExperiments', 'эксперимент: ' + (mem.experiments || []).join(', '));
  setText('memSource', 'Источник: ' + mem.source);
}

function renderNano(nano) {
  const svg = el('nanoChart');
  svg.innerHTML = '';
  const W = 320, H = 140, padL = 34, padB = 22, padT = 10, padR = 8;
  const xMax = 6000, yMax = 0.9;
  const X = (i) => padL + (W - padL - padR) * (i / xMax);
  const Y = (v) => H - padB - (H - padB - padT) * (v / yMax);
  // оси
  svg.appendChild(svgEl('line', { x1: padL, y1: H - padB, x2: W - padR, y2: H - padB, stroke: '#38301f' }));
  svg.appendChild(svgEl('line', { x1: padL, y1: padT, x2: padL, y2: H - padB, stroke: '#38301f' }));
  const mk = (pts, color, dash) => {
    const coords = pts.map((p) => X(p.i) + ',' + Y(p.v)).join(' ');
    svg.appendChild(svgEl('polyline', { points: coords, fill: 'none', stroke: color, 'stroke-width': 2.4, 'stroke-dasharray': dash || 'none' }));
    pts.forEach((p) => {
      svg.appendChild(svgEl('circle', { cx: X(p.i), cy: Y(p.v), r: 3.2, fill: color }));
      const t = svgEl('text', { x: X(p.i), y: Y(p.v) - 5, fill: color, 'font-size': 8, 'text-anchor': 'middle' });
      t.textContent = p.v.toFixed(3);
      svg.appendChild(t);
    });
  };
  const axis = (x, label) => {
    const t = svgEl('text', { x, y: H - 6, fill: '#a89a80', 'font-size': 8, 'text-anchor': 'middle' });
    t.textContent = label; svg.appendChild(t);
  };
  axis(X(0), 'шаг 0'); axis(X(4000), '4000'); axis(X(5999), '~6000');
  mk([{ i: 0, v: 0.3842 }, { i: 5999, v: 0.1483 }], '#f5b301', null);        // train
  mk([{ i: 4000, v: 0.2686 }, { i: 5999, v: 0.8422 }], '#ff8a7a', '5 3');    // val
  const legend = svgEl('text', { x: W - padR, y: padT + 2, fill: '#a89a80', 'font-size': 8, 'text-anchor': 'end' });
  legend.textContent = 'жёлтым — train, красным пунктиром — val';
  svg.appendChild(legend);
  setText('nanoLabel', 'Источник: ' + (nano.label || '—'));
  setText('nanoAnomaly', '⚠ ' + (nano.anomaly || ''));
}

function renderAwakening(list) {
  const tbody = el('awakeningTable').querySelector('tbody');
  tbody.innerHTML = '';
  (list || []).forEach((row) => {
    const tr = document.createElement('tr');
    [row.name, row.params, row.size, row.note].forEach((cell) => {
      const td = document.createElement('td');
      td.textContent = cell;
      tr.appendChild(td);
    });
    tbody.appendChild(tr);
  });
}

function renderStreams(list) {
  const ul = el('streamsList');
  ul.innerHTML = '';
  (list || []).forEach((s) => {
    const li = document.createElement('li');
    const code = document.createElement('code'); code.textContent = s.name;
    li.append(code, document.createTextNode(' — ' + s.what));
    ul.appendChild(li);
  });
}

function deptSearchText(d) {
  return ([d.title, d.id, (d.models || []).join(' '), d.duty].join(' ')).toLowerCase();
}

function renderDepartments(depts) {
  const execRow = el('executiveRow');
  const grid = el('departmentsGrid');
  execRow.innerHTML = ''; grid.innerHTML = '';
  (depts.executive || []).forEach((m) => {
    const card = document.createElement('article'); card.className = 'card';
    const role = document.createElement('h3'); role.textContent = m.role;
    const name = document.createElement('div'); name.style.cssText = 'font-weight:700;margin-bottom:.3rem'; name.textContent = m.name;
    const place = document.createElement('span'); place.className = 'badge'; place.textContent = m.place;
    const duty = document.createElement('p'); duty.className = 'muted small'; duty.style.marginTop = '.5rem'; duty.textContent = m.duty;
    card.append(role, name, place, duty);
    execRow.appendChild(card);
  });
  (depts.departments || []).forEach((d) => {
    const card = document.createElement('article'); card.className = 'card dept-card';
    card.dataset.search = deptSearchText(d);
    const head = document.createElement('h3');
    head.textContent = d.n + '. ' + d.title;
    const room = document.createElement('div'); room.className = 'dept-id'; room.textContent = 'SurrealDB: ' + d.id;
    const mods = document.createElement('ul');
    (d.models || []).forEach((m) => { const li = document.createElement('li'); li.textContent = m; mods.appendChild(li); });
    const duty = document.createElement('p'); duty.className = 'muted small'; duty.textContent = d.duty;
    card.append(head, room, mods, duty);
    grid.appendChild(card);
  });
  setText('deptsProvenance', 'Источник: ' + (depts.provenance || '—'));
}

fetch('/api/state')
  .then((r) => r.json())
  .then((state) => {
    renderRegistry(state.registry);
    renderMemory(state.memory);
    renderNano(state.nano || {});
    renderAwakening(state.awakening);
    renderStreams(state.streams);
    renderDepartments(state.departments || {});
    const stamp = new Date((state.generated_at || 0) * 1000).toLocaleTimeString('ru-RU');
    setText('dashUpdated', '· обновлено ' + stamp);
  })
  .catch(() => {
    setText('dashUpdated', '· данные недоступны: сервер не отвечает');
  });

el('deptFilter').addEventListener('input', (e) => {
  const q = e.target.value.trim().toLowerCase();
  document.querySelectorAll('#departmentsGrid .dept-card').forEach((card) => {
    card.classList.toggle('hidden', q && !card.dataset.search.includes(q));
  });
});
