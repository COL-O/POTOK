#!/usr/bin/env python3
"""«Окно дома» — единая точка входа в проект ПОТОК и в этот ноутбук.

Одно окно, две створки:
  Дом     — ноутбук (нагрузка, память, диск, температура), службы, порты;
  Проект  — организм ПОТОКА, прогоны CI, события, доска, чат, зрение (O2).

Данные собираются здесь же: systemd, /proc, порты, REST OpenObserve, :8080.
Никаких внешних зависимостей — только стандартная библиотека.

Запуск: python3 okno.py [--port 8799]
"""
import base64
import email.utils as email_utils
import json
import os
import pathlib
import shutil
import socket
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# Всё настраивается средой: у каждого смотрителя свои порты и пути.
PORT = int(os.environ.get("OKNO_PORT", "8799"))
O2 = os.environ.get("OKNO_O2", "http://127.0.0.1:5080")
POTOK = os.environ.get("OKNO_POTOK", "http://127.0.0.1:8080")
SECRET = pathlib.Path(os.environ.get(
    "OKNO_SECRET", str(pathlib.Path.home() / ".config/domovoy/secrets/openobserve-root.txt")))
GH = os.environ.get("OKNO_GH") or (shutil.which("gh") or "/home/colo/go/bin/gh")
REPO = os.environ.get("OKNO_REPO", "COL-O/POTOK")
CHAT = int(os.environ.get("OKNO_CHAT", "12"))

UNITS = {
    "openobserve": "зрение (O2)",
    "o2-feed.timer": "питание зрения",
    "graphiti-memory": "память (Graphiti)",
    "poligon-models": "модели (pulsar)",
    "poligon-svc": "полки/слои",
    "minilm-embed": "эмбеддинги minilm",
    "rubert-embed": "эмбеддинги rubert",
    "silero-tts": "голос (TTS)",
    "voice-input": "слух (STT)",
    "needle-shift": "помощник needle",
    "shadow-shift": "помощник shadow",
    "domovoy-fast": "помощник domovoy-fast",
    "clickhouse": "аналитика ClickHouse",
    "kanr-ag2http": "полководец AG2",
    "potok-brief.timer": "утренняя сводка",
    "colab-keepalive": "Colab",
    "dom-okno": "окно дома",
}

PORTS = {
    5080: "O2 http", 8529: "ArangoDB", 8123: "ClickHouse", 8791: "Graphiti",
    8101: "pulsar", 1237: "minilm", 1240: "rubert", 8700: "TTS", 8702: "STT",
    8792: "слои", 8798: "domovoy-fast", 8799: "окно дома", 8080: "ПОТОК",
}

LINKS = {
    "Дашборд организма": "/web/dashboards/view?org_identifier=default&dashboard=7529073235594313728&folder=default",
    "Доска «Круглый стол»": "https://github.com/users/COL-O/projects/5",
    "Чат (обсуждение №12)": "https://github.com/COL-O/POTOK/discussions/12",
    "Задача-ветка дома": "https://github.com/COL-O/POTOK/issues/10",
    "Ветка Притока": "https://github.com/COL-O/POTOK/issues/11",
    "PR №7 (дом)": "https://github.com/COL-O/POTOK/pull/7",
    "PR №9 (Приток)": "https://github.com/COL-O/POTOK/pull/9",
}

_cache = {"ts": 0, "o2": {}, "ci": []}
_proj = {"ts": 0, "data": {}}
_lock = threading.Lock()


def o2_auth() -> str:
    lines = SECRET.read_text(encoding="utf-8").splitlines()
    return base64.b64encode(f"{lines[0]}:{lines[1]}".encode()).decode()


def o2_sql(sql: str, minutes: int = 1440) -> list:
    now = int(time.time())
    body = json.dumps({"query": {"sql": sql,
                                 "start_time": (now - minutes * 60) * 1_000_000,
                                 "end_time": (now + 120) * 1_000_000}}).encode()
    req = urllib.request.Request(f"{O2}/api/default/_search?type=logs", data=body, method="POST",
                                 headers={"Authorization": f"Basic {o2_auth()}",
                                          "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=8) as r:
        return json.loads(r.read().decode()).get("hits", [])


def get_json(url: str, timeout: float = 3.0):
    with urllib.request.urlopen(url, timeout=timeout) as r:
        return json.loads(r.read().decode())


def unit_state(name: str) -> str:
    try:
        r = subprocess.run(["systemctl", "--user", "is-active", name],
                           capture_output=True, text=True, timeout=8)
        return (r.stdout or "").strip() or "unknown"
    except Exception:
        return "unknown"


def port_open(port: int) -> bool:
    with socket.socket() as s:
        s.settimeout(0.3)
        return s.connect_ex(("127.0.0.1", port)) == 0


def bases_block() -> dict:
    """Общие базы проекта: живы ли и сколько в них лежит."""
    out = {}

    def http(url, timeout=2.5):
        try:
            with urllib.request.urlopen(url, timeout=timeout) as r:
                return r.status
        except Exception as e:  # noqa: BLE001
            return getattr(e, "code", 0) or 0

    out["arangodb"] = "ok" if http("http://127.0.0.1:8529/_api/version") in (200, 401) else "нет"
    out["clickhouse"] = "ok" if http("http://127.0.0.1:8123/ping") == 200 else "нет"
    out["graphiti"] = "ok" if http("http://127.0.0.1:8791/health") == 200 else "нет"
    out["openobserve"] = "ok" if http(f"{O2}/healthz") == 200 else "нет"
    for name, path in (("duckdb", "/home/colo/.local/share/duck.db"),
                       ("opencode.db", "/home/colo/.local/share/opencode/opencode.db")):
        p = pathlib.Path(path)
        out[name] = f"{p.stat().st_size / 1e6:.1f} МБ" if p.exists() else "нет"
    return out


def host_block() -> dict:
    mem = {}
    for line in pathlib.Path("/proc/meminfo").read_text().splitlines():
        k, v = line.split(":", 1)
        mem[k] = int(v.strip().split()[0])
    du = shutil.disk_usage("/")
    try:
        temp = int(pathlib.Path("/sys/class/thermal/thermal_zone0/temp").read_text()) / 1000
    except Exception:
        temp = 0
    load = os.getloadavg()
    return {
        "load": [round(x, 2) for x in load],
        "mem_total": mem.get("MemTotal", 0) // 1024,
        "mem_avail": mem.get("MemAvailable", 0) // 1024,
        "swap_used": (mem.get("SwapTotal", 0) - mem.get("SwapFree", 0)) // 1024,
        "disk_total": round(du.total / 1e9, 1),
        "disk_free": round(du.free / 1e9, 1),
        "temp": round(temp, 1),
        "uptime_h": round(float(pathlib.Path("/proc/uptime").read_text().split()[0]) / 3600, 1),
        "cpu": os.cpu_count(),
        "host": socket.gethostname(),
    }


def o2_block() -> dict:
    """Тяжёлая часть — кэш 15 секунд."""
    with _lock:
        if time.time() - _cache["ts"] < 15:
            return {"events": _cache["o2"], "ci": _cache["ci"]}
        out, ci = {}, []
        try:
            out["events24h"] = o2_sql("select count(*) as n from agent_events")[0]["n"]
            out["ticks"] = o2_sql("select count(*) as n from potok_metrics")[0]["n"]
            out["host_points"] = o2_sql("select count(*) as n from host_metrics")[0]["n"]
            out["services"] = o2_sql(
                "select name, title, state from services_health order by _timestamp desc", 30)
            ci = o2_sql("select workflow, conclusion, branch, created_at from ci_runs "
                        "order by created_at desc limit 8")
            out["last_events"] = o2_sql(
                "select kind, action, result from agent_events order by _timestamp desc limit 5")
        except Exception as e:  # noqa: BLE001
            out["error"] = str(e)[:120]
        _cache.update(ts=time.time(), o2=out, ci=ci)
        return {"events": out, "ci": ci}


def potok_block() -> dict:
    try:
        h = get_json(f"{POTOK}/health")
        m = get_json(f"{POTOK}/metrics")
        return {"up": True, "health": h, "metrics": m}
    except Exception:
        return {"up": False}


def gh_json(args: list, timeout: int = 25):
    try:
        r = subprocess.run([GH] + args, capture_output=True, text=True, timeout=timeout)
        if r.returncode != 0:
            return None
        return json.loads(r.stdout or "null")
    except Exception:  # noqa: BLE001
        return None


def project_block() -> dict:
    """Задачи, проверки, доска — то, чем управляют. Кэш 60 с: gh не любит частоту."""
    global _proj
    with _lock:
        if time.time() - _proj["ts"] < 60 and _proj["data"]:
            return _proj["data"]
    issues = gh_json(["issue", "list", "--repo", REPO, "--state", "open", "--limit", "20",
                      "--json", "number,title,labels,updatedAt"]) or []
    prs = gh_json(["pr", "list", "--repo", REPO, "--state", "open", "--limit", "10",
                   "--json", "number,title,headRefName,isDraft,reviewDecision"]) or []
    runs = gh_json(["run", "list", "--repo", REPO, "--limit", "5",
                    "--json", "workflowName,status,conclusion,headBranch,databaseId,createdAt"]) or []
    data = {
        "issues": [{"n": x.get("number"), "title": x.get("title", ""),
                    "labels": [l.get("name") for l in (x.get("labels") or [])][:3],
                    "updated": x.get("updatedAt", "")} for x in issues],
        "prs": [{"n": x.get("number"), "title": x.get("title", ""),
                 "branch": x.get("headRefName", ""), "draft": x.get("isDraft", False)}
                for x in prs],
        "runs": [{"id": x.get("databaseId"), "workflow": x.get("workflowName", ""),
                  "branch": x.get("headBranch", ""), "status": x.get("status", ""),
                  "conclusion": x.get("conclusion") or "", "at": x.get("createdAt", "")}
                 for x in runs],
    }
    with _lock:
        _proj.update(ts=time.time(), data=data)
    return data


def do_action(payload: dict) -> dict:
    """Пульт: только белый список действий. Никаких произвольных команд."""
    what = (payload or {}).get("do", "")
    if what == "unit":
        name = str(payload.get("name", ""))
        op = str(payload.get("op", ""))
        if name not in UNITS or op not in ("start", "stop", "restart"):
            return {"ok": False, "error": "служба или действие вне белого списка"}
        r = subprocess.run(["systemctl", "--user", op, name], capture_output=True,
                           text=True, timeout=30)
        return {"ok": r.returncode == 0, "out": (r.stdout or r.stderr).strip()[:200],
                "state": unit_state(name)}
    if what == "chat":
        text = str(payload.get("text", "")).strip()
        if not text or len(text) > 4000:
            return {"ok": False, "error": "текст пуст или длиннее 4000 символов"}
        owner, name = REPO.split("/", 1)
        rid = gh_json(["api", "graphql", "-f",
                       'query($o:String!,$n:String!,$num:Int!){repository(owner:$o,name:$n){discussion(number:$num){id}}}',
                       "-f", f"o={owner}", "-f", f"n={name}", "-F", f"num={CHAT}"])
        did = ((((rid or {}).get("data") or {}).get("repository") or {})
               .get("discussion") or {}).get("id")
        if not did:
            return {"ok": False, "error": f"обсуждение №{CHAT} не найдено"}
        res = gh_json(["api", "graphql", "-f",
                       "mutation($id:ID!,$b:String!){addDiscussionComment("
                       "input:{discussionId:$id,body:$b}){comment{url}}}",
                       "-f", f"id={did}", "-f", f"b={text}"])
        url = ((((res or {}).get("data") or {}).get("addDiscussionComment") or {})
               .get("comment") or {}).get("url", "")
        return {"ok": bool(url), "url": url,
                "error": "" if url else str((res or {}).get("errors", ""))[:200]}
    if what == "brief":
        r = subprocess.run(["systemctl", "--user", "start", "potok-brief.service"],
                           capture_output=True, text=True, timeout=120)
        return {"ok": r.returncode == 0, "out": (r.stdout or r.stderr).strip()[:200]}
    if what == "refresh":
        with _lock:
            _cache.update(ts=0, o2={}, ci=[])
            _proj.update(ts=0, data={})
        return {"ok": True}
    return {"ok": False, "error": "неизвестное действие"}


def state() -> dict:
    units = [{"name": n, "title": t, "state": unit_state(n)} for n, t in UNITS.items()]
    ports = [{"port": p, "title": t, "open": port_open(p)} for p, t in PORTS.items()]
    return {
        "ts": int(time.time()),
        "host": host_block(),
        "units": units,
        "ports": ports,
        "potok": potok_block(),
        "project": project_block(),
        "o2": o2_block(),
        "bases": bases_block(),
        "repo": REPO,
        "links": LINKS,
    }


HTML = """<!doctype html><html lang="ru"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>ДОМОВОЙ · окно дома</title>
<style>
:root{--bg:#0e1116;--panel:#161b22;--line:#232a34;--tx:#e6edf3;--dim:#8b949e;
--ok:#3fb950;--bad:#f85149;--warn:#d29922;--acc:#58a6ff}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--tx);font:14px/1.45 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
header{display:flex;align-items:center;gap:16px;padding:14px 20px;border-bottom:1px solid var(--line);position:sticky;top:0;background:#0e1116ee;backdrop-filter:blur(6px);z-index:5}
h1{font-size:16px;margin:0;font-weight:600;letter-spacing:.2px}
h1 span{color:var(--dim);font-weight:400}
nav{display:flex;gap:6px;margin-left:auto}
button.tab{background:transparent;border:1px solid var(--line);color:var(--dim);padding:6px 14px;border-radius:8px;cursor:pointer;font-size:13px}
button.tab.on{background:#1f6feb22;border-color:#1f6feb88;color:var(--acc)}
main{padding:16px 20px 40px;display:none}
main.on{display:block}
.grid{display:grid;gap:12px;grid-template-columns:repeat(auto-fit,minmax(260px,1fr))}
.card{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:14px 16px}
.card h3{margin:0 0 10px;font-size:13px;color:var(--dim);font-weight:500;text-transform:uppercase;letter-spacing:.6px}
.big{font-size:26px;font-weight:600;line-height:1.1}
.sub{color:var(--dim);font-size:12px;margin-top:4px}
.row{display:flex;justify-content:space-between;gap:10px;padding:5px 0;border-bottom:1px dashed #1e242d}
.row:last-child{border-bottom:0}
.dot{width:8px;height:8px;border-radius:50%;display:inline-block;margin-right:7px}
.ok{background:var(--ok)}.bad{background:var(--bad)}.warn{background:var(--warn)}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px}
a{color:var(--acc);text-decoration:none}a:hover{text-decoration:underline}
ul.links{list-style:none;margin:0;padding:0}
ul.links li{padding:4px 0}
iframe{width:100%;height:78vh;border:1px solid var(--line);border-radius:12px;background:#fff}
.act{background:#1f6feb22;border:1px solid #1f6feb88;color:var(--acc);padding:5px 11px;border-radius:8px;cursor:pointer;font-size:12px}
.act:hover{background:#1f6feb33}
.act.primary{background:#23863622;border-color:#23863688;color:var(--ok)}
.act.warn{background:#bb800922;border-color:#bb800988;color:var(--warn)}
.badge{font-size:11px;padding:2px 8px;border-radius:999px;border:1px solid var(--line);color:var(--dim)}
.tail{color:var(--dim);font-size:12px;margin-top:8px}
</style></head><body>
<header><h1>ДОМОВОЙ <span>· окно дома</span></h1>
<span class="badge" id="clock">—</span>
<nav><button class="tab on" data-t="dom">Дом</button>
<button class="tab" data-t="proj">Проект ПОТОК</button>
<button class="tab" data-t="uprav">Управление</button></nav></header>

<main id="dom" class="on">
 <div class="grid">
  <div class="card"><h3>Ноутбук</h3>
   <div class="big" id="load">—</div><div class="sub" id="hostline">—</div>
   <div class="row"><span>Память</span><span class="mono" id="mem">—</span></div>
   <div class="row"><span>Диск</span><span class="mono" id="disk">—</span></div>
   <div class="row"><span>Температура</span><span class="mono" id="temp">—</span></div>
   <div class="row"><span>Вверх</span><span class="mono" id="uptime">—</span></div>
  </div>
  <div class="card"><h3>Службы дома</h3><div id="units"></div></div>
  <div class="card"><h3>Порты</h3><div id="ports"></div></div>
  <div class="card"><h3>Зрение (O2)</h3>
   <div class="row"><span>События за сутки</span><span class="mono" id="ev">—</span></div>
   <div class="row"><span>Точки хоста</span><span class="mono" id="hp">—</span></div>
   <div class="row"><span>Срезы организма</span><span class="mono" id="tk">—</span></div>
   <div class="row"><span>Состояние</span><span class="mono" id="o2ok">—</span></div>
  </div>
  <div class="card"><h3>Общие базы</h3><div id="bases"></div></div>
 </div>
</main>

<main id="proj">
 <div class="grid">
  <div class="card"><h3>Организм</h3>
   <div class="big" id="pstate">—</div><div class="sub" id="pdetail">—</div></div>
  <div class="card"><h3>Иммунитет (CI)</h3><div id="ci" class="mono">—</div></div>
  <div class="card"><h3>Задачи в работе</h3><div id="tasks" class="mono">—</div></div>
  <div class="card"><h3>Ветки и PR</h3><div id="prs" class="mono">—</div></div>
  <div class="card"><h3>Последние события</h3><div id="events" class="mono">—</div></div>
  <div class="card"><h3>Куда идти</h3><ul class="links" id="links"></ul></div>
 </div>
 <div class="card" style="margin-top:12px"><h3>Дашборд «Состояние организма» (OpenObserve)</h3>
  <iframe id="dash" data-src="/web/dashboards/view?org_identifier=default&dashboard=7529073235594313728&folder=default" loading="lazy"></iframe>
  <div class="tail">Вход в OpenObserve выполняется сам при открытии окна. Если дашборд пуст — нажми <a href="#" onclick="o2login(true);return false">войти заново</a>.</div>
 </div>
</main>

<main id="uprav">
 <div class="grid">
  <div class="card"><h3>Службы дома — кнопки</h3><div id="units-ctl"></div></div>
  <div class="card"><h3>Действия по проекту</h3>
   <div class="sub" style="margin-bottom:8px">Реплика в общий чат (обсуждение №12)</div>
   <textarea id="chat-text" rows="5" placeholder="Коротко и по делу…"
     style="width:100%;background:#0e1116;color:var(--tx);border:1px solid var(--line);border-radius:8px;padding:8px;font:13px/1.4 inherit"></textarea>
   <div style="display:flex;gap:8px;margin-top:8px;flex-wrap:wrap">
    <button class="act primary" onclick="act('chat')">Отправить в чат</button>
    <button class="act" onclick="act('brief')">Опубликовать сводку</button>
    <button class="act" onclick="act('refresh')">Обновить данные</button>
   </div>
   <div class="tail" id="act-result">—</div>
  </div>
  <div class="card"><h3>Проверки (последние)</h3><div id="runs" class="mono">—</div></div>
 </div>
</main>

<script>
const $ = id => document.getElementById(id);
// Автовход в OpenObserve: куку ставит сам портал (пароль остаётся на сервере).
async function o2login(force){
  try{
    const r = await fetch('/api/o2-login', {method:'POST'});
    const j = await r.json().catch(()=>({}));
    if (j && j.userInfo) {
      try {
        localStorage.setItem('userInfo', j.userInfo);
        if (j.currentuser) localStorage.setItem('currentuser', j.currentuser);
      } catch(e){}
    }
    const f = $('dash');
    if (f && f.dataset.src) {
      if (force || !f.src) f.src = f.dataset.src;
      else f.src = f.dataset.src + '&_r=' + Date.now();
    }
    return j;
  }catch(e){ return {}; }
}
o2login(false);
document.querySelectorAll('button.tab').forEach(b => b.onclick = () => {
  document.querySelectorAll('button.tab').forEach(x => x.classList.remove('on'));
  document.querySelectorAll('main').forEach(x => x.classList.remove('on'));
  b.classList.add('on'); $(b.dataset.t).classList.add('on');
});
const dot = ok => `<span class="dot ${ok ? 'ok' : 'bad'}"></span>`;
function render(s){
  $('clock').textContent = new Date(s.ts*1000).toLocaleTimeString('ru-RU');
  const h = s.host;
  $('load').textContent = h.load.join(' · ');
  $('hostline').textContent = `${h.host} · ${h.cpu} ядер · load 1/5/15`;
  $('mem').textContent = `${h.mem_total - h.mem_avail} / ${h.mem_total} МБ`;
  $('disk').textContent = `${(h.disk_total-h.disk_free).toFixed(1)} / ${h.disk_total} ГБ (свободно ${h.disk_free})`;
  $('temp').textContent = h.temp ? h.temp + ' °C' : '—';
  $('uptime').textContent = h.uptime_h + ' ч';
  $('units').innerHTML = s.units.map(u => `<div class="row">${dot(u.state==='active')}<span style="flex:1">${u.title}</span><span class="mono">${u.state}</span></div>`).join('');
  $('ports').innerHTML = s.ports.map(p => `<div class="row">${dot(p.open)}<span style="flex:1">${p.title}</span><span class="mono">${p.port}</span></div>`).join('');
  const o = s.o2.events || {};
  $('ev').textContent = o.events24h ?? '—';
  $('hp').textContent = o.host_points ?? '—';
  $('tk').textContent = o.ticks ?? '—';
  $('o2ok').textContent = o.error ? 'ошибка: ' + o.error : 'ok';
  const b = s.bases || {};
  $('bases').innerHTML = Object.entries(b).map(([k,v]) =>
    `<div class="row">${v === 'нет' ? '<span class="dot bad"></span>' : '<span class="dot ok"></span>'}
     <span style="flex:1">${k}</span><span class="mono">${v}</span></div>`).join('');
  const p = s.potok || {};
  $('pstate').innerHTML = p.up ? '<span style="color:var(--ok)">живой</span>' : '<span style="color:var(--dim)">не поднят</span>';
  $('pdetail').textContent = p.up ? `тактов: ${p.health.published_ticks ?? '—'} · ошибок границ: ${p.health.bounds_errors ?? 0}` : 'поднять: ORGANISM_DAEMON=1 go run ./cmd/organism/...';
  const ci = s.o2.ci || [];
  $('ci').innerHTML = ci.length ? ci.map(c => `${dot(c.conclusion==='success')}<span class="mono">${(c.workflow||'').slice(0,26)} · ${c.conclusion||'—'} · ${c.branch||''}</span>`).join('<br>') : '—';
  const ev = o.last_events || [];
  $('events').innerHTML = ev.length ? ev.map(e => `<div>${dot(false)}<b>${(e.kind||'').slice(0,60)}</b><div class="sub">${(e.action||'').slice(0,120)}</div></div>`).join('') : '—';
  $('links').innerHTML = Object.entries(s.links).map(([k,v]) => `<li><a href="${v}" target="_blank">${k}</a></li>`).join('');
  const pr = s.project || {};
  const iss = pr.issues || [];
  $('tasks').innerHTML = iss.length ? iss.map(i =>
    `<div class="row"><span style="flex:1"><a href="https://github.com/${s.repo||'COL-O/POTOK'}/issues/${i.n}" target="_blank">#${i.n}</a> ${(i.title||'').slice(0,40)}</span>` +
    `<span class="mono">${(i.labels||[]).join(' ').slice(0,14)}</span></div>`).join('') : '—';
  const prs = pr.prs || [];
  $('prs').innerHTML = prs.length ? prs.map(x =>
    `<div class="row">${dot(!x.draft)}<span style="flex:1"><a href="https://github.com/${s.repo||'COL-O/POTOK'}/pull/${x.n}" target="_blank">#${x.n}</a> ${(x.title||'').slice(0,36)}</span>` +
    `<span class="mono">${(x.branch||'').slice(-18)}</span></div>`).join('') : '—';
  const runs = pr.runs || [];
  $('runs').innerHTML = runs.length ? runs.map(r =>
    `${dot(r.conclusion==='success')}<span class="mono">${(r.workflow||'').slice(0,22)} · ${r.conclusion||r.status||'—'} · ${(r.branch||'').slice(-16)}</span>`).join('<br>') : '—';
  $('units-ctl').innerHTML = s.units.map(u =>
    `<div class="row">${dot(u.state==='active')}<span style="flex:1">${u.title}</span>
     <button class="act ${u.state==='active'?'warn':'primary'}" onclick="act('unit','${u.name}','${u.state==='active'?'restart':'start'}')">${u.state==='active'?'перезапуск':'старт'}</button>
     ${u.state==='active'?`<button class="act" onclick="act('unit','${u.name}','stop')">стоп</button>`:''}
    </div>`).join('');
}

async function act(what, name, op){
  const out = $('act-result');
  if (what === 'chat') {
    const text = ($('chat-text').value || '').trim();
    if (!text) { out.textContent = 'пусто: напиши текст'; return; }
    out.textContent = 'отправляю…';
    const r = await fetch('/api/action', {method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({do:'chat', text})}).then(r=>r.json()).catch(e=>({ok:false,error:String(e)}));
    if (r.ok) { out.textContent = 'отправлено: ' + (r.url||''); $('chat-text').value = ''; }
    else out.textContent = 'не отправлено: ' + (r.error||'ошибка');
    tick(); return;
  }
  out.textContent = 'выполняю…';
  const r = await fetch('/api/action', {method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify({do:what, name:name||'', op:op||''})}).then(r=>r.json()).catch(e=>({ok:false,error:String(e)}));
  out.textContent = r.ok ? (what==='brief' ? 'сводка запущена' : what==='refresh' ? 'данные обновлены' : `${name}: ${r.state||'готово'}`) : 'ошибка: ' + (r.error||'');
  tick();
}
async function tick(){
  try { render(await (await fetch('/api/state')).json()); } catch(e){ $('clock').textContent = 'нет связи'; }
}
tick(); setInterval(tick, 10000);
</script></body></html>"""


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):  # тишина в журнале
        pass

    def _send(self, code: int, body: bytes, ctype: str, extra: list | None = None):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        for k, v in (extra or []):
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(body)

    def _login(self):
        """Автовход в O2: кука + запись userInfo, чтобы панель не уводила на логин."""
        lines = SECRET.read_text(encoding="utf-8").splitlines()
        user_email = lines[0]
        body = json.dumps({"name": user_email, "password": lines[1]}).encode()
        req = urllib.request.Request(f"{O2}/auth/login", data=body, method="POST",
                                     headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=10) as r:
                payload = r.read()
                cookies = [(k, v) for k, v in r.headers.items() if k.lower() == "set-cookie"]
        except Exception as e:  # noqa: BLE001
            self._send(502, json.dumps({"ok": False, "error": str(e)[:160]}).encode(),
                       "application/json")
            return
        # панель O2 хранит текущего пользователя в localStorage — собираем то же самое
        exp = int(time.time()) + 29 * 86400
        for k, v in cookies:
            for part in v.split(";"):
                if part.strip().lower().startswith("expires="):
                    try:
                        exp = int(email_utils.parsedate_to_datetime(part.split("=", 1)[1].strip()).timestamp())
                    except Exception:  # noqa: BLE001
                        pass
        user = {"given_name": user_email, "auth_time": int(time.time()), "name": user_email,
                "exp": exp, "family_name": "", "email": user_email}
        raw = json.dumps(user, separators=(",", ":")).encode()
        out = {"status": True, "message": "",
               "userInfo": base64.b64encode(raw).decode(),
               "currentuser": json.dumps(json.dumps(user, separators=(",", ":")))}
        self._send(200, json.dumps(out).encode(), "application/json", cookies)

    def _proxy(self):
        """Всё, что не наше, — прозрачно в OpenObserve: куки, методы, тело."""
        length = int(self.headers.get("Content-Length") or 0)
        data = self.rfile.read(length) if length else None
        headers = {"Accept": self.headers.get("Accept", "*/*")}
        for h in ("Cookie", "Authorization", "Content-Type", "Accept-Language",
                  "Origin", "Referer", "X-Requested-With"):
            v = self.headers.get(h)
            if v:
                headers[h] = v
        req = urllib.request.Request(O2 + self.path, data=data, method=self.command,
                                     headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=25) as r:
                payload = r.read()
                ctype = r.headers.get("Content-Type", "application/octet-stream")
                extra = [(k, v) for k, v in r.headers.items()
                         if k.lower() in ("set-cookie", "location", "content-disposition")]
                self._send(r.status, payload, ctype, extra)
        except urllib.error.HTTPError as e:
            payload = e.read() or b""
            ctype = e.headers.get("Content-Type", "text/plain")
            extra = [(k, v) for k, v in e.headers.items() if k.lower() == "set-cookie"]
            self._send(e.code, payload, ctype, extra)
        except Exception:
            self._send(502, "openobserve недоступен".encode(), "text/plain")

    def _route(self):
        path = self.path.split("?")[0]
        if path == "/api/o2-login":
            self._login()
        elif path == "/api/action":
            length = int(self.headers.get("Content-Length") or 0)
            try:
                payload = json.loads(self.rfile.read(length).decode() or "{}")
                out = do_action(payload)
            except Exception as e:  # noqa: BLE001
                out = {"ok": False, "error": str(e)[:160]}
            self._send(200, json.dumps(out, ensure_ascii=False).encode(), "application/json")
        elif path == "/api/state":
            try:
                self._send(200, json.dumps(state(), ensure_ascii=False).encode(), "application/json")
            except Exception as e:  # noqa: BLE001
                self._send(500, json.dumps({"error": str(e)[:200]}).encode(), "application/json")
        elif path in ("/", "/index.html"):
            self._send(200, HTML.encode(), "text/html; charset=utf-8")
        else:
            self._proxy()

    def do_GET(self):
        self._route()

    def do_POST(self):
        self._route()

    def do_PUT(self):
        self._route()

    def do_DELETE(self):
        self._route()

    def do_PATCH(self):
        self._route()


def main() -> int:
    port = PORT
    if "--port" in sys.argv:
        port = int(sys.argv[sys.argv.index("--port") + 1])
    srv = ThreadingHTTPServer(("127.0.0.1", port), Handler)
    print(f"окно дома: http://127.0.0.1:{port}/")
    srv.serve_forever()
    return 0


if __name__ == "__main__":
    sys.exit(main())
