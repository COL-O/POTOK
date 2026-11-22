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

/* ---------- O2-контур (сверка с openobserve.ai, 2026-11-22; на живом O2 не проверялось) ---------- */
const O2_ITEMS = [
  { s: 'ok', t: 'JSON ingest · POST /api/default/<stream>/_json', d: 'Штатный вход для скриптов, резидентов и резидентных отчётов.' },
  { s: 'ok', t: 'Потоки создаются при первом событии', d: 'society_events / society_roster / ai_consensus / system_docs — создавать заранее не нужно.' },
  { s: 'ok', t: 'OTLP · /api/default/v1/traces|metrics|logs', d: 'Стандартный канал для Python-скриптов и llama-server; коллектор не обязателен; trailing slash нельзя.' },
  { s: 'ok', t: 'VRL Pipelines (real-time)', d: 'Трансформация на приёме на скорости Rust: snake_case, признак wake_needed; без выделения ОЗУ.' },
  { s: 'ok', t: 'Enrichment Tables (CSV)', d: 'Pipelines > Enrichment Tables; lookup функцией get_enrichment_table_record() в VRL; для SQL-джойна префикс enrich.' },
  { s: 'cond', t: 'Remote Tasks · /api/{org}/tasks', d: 'Существует, но это задача оценки экспериментов: синхронный HTTP, Timeout ≤ 600 с (по умолч. 60), до 3 попыток, HMAC-SHA256 в x-o2-signature. Enterprise + O2_EVAL_ENABLED. НЕ асинхронный мост в Colab через Google Диск.' },
  { s: 'cond', t: 'Providers (Settings) · до 7 типов', d: 'В т. ч. openai_compatible; endpoint + модели + ключ. Ваш lfm2.5-350m-local (:8088) сюда; виден только если AI suite включён (редакция/флаги).' },
  { s: 'cond', t: 'Playground', d: 'Варианты × кейсы; temperature/max_tokens/tool calling; volatile (ничего не сохраняет); promote в Dataset/Experiment. Enterprise.' },
  { s: 'cond', t: 'Prompts · реестр с версиями и метками', d: 'v1/v2, label «production» указывает на нужную версию; RBAC; трафик считается по трейсам. Enterprise.' },
  { s: 'cond', t: 'Datasets / Experiments', d: 'Версионируемые кейсы (input/expected_output), pinned snapshot, baseline-сравнение, стоимость считается и подтверждается. Enterprise.' },
  { s: 'cond', t: 'CSV-загрузка файлов из браузера', d: 'Для enrichment tables подтверждена; для произвольных потоков — только в вашем UI проверить.' },
  { s: 'no', t: 'Remote Task → ноутбук → Диск → Colab как «мост»', d: 'Не сходится: Remote Task — синхронный HTTP ≤ 600 с, контур через Drive асинхронен (минуты). Пути: SDK task (ваш процесс репортит результат) или вердикт напрямую в поток ai_consensus.' },
  { s: 'no', t: 'Агрегации (GROUP BY) внутри VRL', d: 'VRL — per-event трансформация; сводки — SQL, reports, scheduled pipeline. Выдумать синтаксис нельзя.' },
  { s: 'no', t: '«Длинные логи до 256K контекста»', d: 'Нет ни у одной модели room_resident: SmolLM2-360M 8K, Qwen2.5-0.5B 32K, Gemma-3-270M 32K, LFM2.5-350M 32K (HF).' },
  { s: 'no', t: 'Серверные бинарники O2 в релизах GitHub', d: 'В релизе v1.2.0-rc2 — только openapi.json; поставка: Docker / Cloud / Enterprise. В эту песочницу O2 поставить нельзя (docker нет, Rust-тулчейн вне доступа).' },
];
const O2_VERDICT = [
  'Remote Tasks — инструмент оценки экспериментов, а не нервная система и не «главный мост» к Colab; для 7B-контура — SDK task или поток ai_consensus.',
  'Имена и лимиты в документе в целом верны; смысл искажён: O2 — это телеметрия и оценка, а не исполнитель задач.',
  'Всё AI-бюджетное (Providers/Playground/Prompts/Datasets/Experiments) — Enterprise / Cloud с флагом O2_EVAL_ENABLED; наличие на вашем v1.1.2 устанавливает только смотритель.',
  'Всё, что я «поставил в проект», — черновики в o2/amber-society/ и эта вкладка; живые машины не трогал.',
];
const O2_STATUS = { ok: '✅', cond: '◐', no: '✖' };
const o2Grid = el('o2Grid');
if (o2Grid) {
  O2_ITEMS.forEach((item) => {
    const card = document.createElement('div');
    card.className = 'o2-item s-' + item.s;
    const h = document.createElement('h4');
    const badge = document.createElement('span'); badge.className = 'badge2'; badge.textContent = O2_STATUS[item.s];
    h.append(badge, document.createTextNode(item.t));
    const p = document.createElement('p'); p.textContent = item.d;
    card.append(h, p);
    o2Grid.appendChild(card);
  });
  const verdict = el('o2Verdict');
  O2_VERDICT.forEach((line) => { const li = document.createElement('li'); li.textContent = line; verdict.appendChild(li); });
}
