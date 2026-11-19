#!/usr/bin/env node
// Проверка воркфлоу GitHub Actions и конфига линтера — до пуша, а не после.
//
// Зачем. Воркфлоу, скопированный из подсказки или из чужого проекта, ломается
// тихо: несуществующий тег экшена даёт красный прогон с невнятной ошибкой,
// забытая ветка — просто не запускает проверку, а неверная версия схемы
// линтера — «unknown field». Всё это ловится чтением, без запуска.
//
// Что проверяется:
//   1. синтаксис YAML и базовая структура (jobs, runs-on, steps);
//   2. `uses:` — репозиторий существует и у него есть такой тег/ветка (через gh);
//      локальные `./.github/...` и docker:// пропускаются;
//   3. `on:` — покрывает ли воркфлоу ветку, на которой я стою (частая поломка:
//      работаешь в arena/**, а воркфлоу слушает только main);
//   4. шаги: `uses` ИЛИ `run`, не оба пусто; у `run` не забыт shell для пайпов;
//   5. для golangci-lint: если конфиг схемы v2 — экшен должен быть v7+ (v6 максимум v1.64),
//      а версия линтера — v2.x;
//   6. `.golangci.yml` — ключи верхнего уровня из схемы v2, никаких v1-остатков
//      (disable-all, enable-all, presets, fast, run.skip-dirs…).
//
// Границей честно: это чтение объявлений. Что прогон действительно пройдёт —
// подтверждает только прогон.
//
// Использование:
//   node check.mjs <корень> [--offline] [--branch arena/...]

import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import * as yaml from 'js-yaml';

const args = process.argv.slice(2);
const root = path.resolve(args[0] || '.');
const offline = args.includes('--offline');
const branchArg = (() => {
  const i = args.indexOf('--branch');
  return i >= 0 ? args[i + 1] : null;
})();

const problems = [];
const warnings = [];
const notes = [];
const bad = (f, m) => problems.push(`${f}: ${m}`);
const warn = (f, m) => warnings.push(`${f}: ${m}`);

// v1-ключи, которых в схеме v2 нет: их наличие в конфиге = «unknown field» при прогоне
const V1_ONLY = [
  'disable-all', 'enable-all', 'presets', 'fast', 'exclude-use-default',
  'exclude-dirs', 'exclude-dirs-use-default', 'exclude-files', 'exclude-rules',
  'skip-dirs', 'skip-files', 'skip-dirs-use-default', 'modules-download-mode',
];
const V2_TOP = new Set(['version', 'run', 'output', 'linters', 'formatters', 'issues', 'severity']);

function ghOk(endpoint) {
  try {
    execFileSync('gh', ['api', endpoint], { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

function tagExists(actionRepo, ref) {
  // ref может быть веткой (master/main) или тегом
  return ghOk(`repos/${actionRepo}/git/ref/tags/${ref}`) || ghOk(`repos/${actionRepo}/git/ref/heads/${ref}`);
}

// --- 1. структура ---------------------------------------------------------

function checkStructure(file, doc) {
  if (!doc || typeof doc !== 'object' || Array.isArray(doc)) {
    bad(file, 'не читается как YAML-объект');
    return false;
  }
  // js-yaml превращает «on:» в булев ключ true (YAML 1.1) — это известная ловушка
  const on = doc.on ?? doc[true];
  if (!on) bad(file, 'нет блока `on:` — воркфлоу никогда не запустится');
  if (!doc.jobs || typeof doc.jobs !== 'object') {
    bad(file, 'нет `jobs:`');
    return false;
  }
  for (const [id, job] of Object.entries(doc.jobs)) {
    if (!job || typeof job !== 'object') return bad(file, `job ${id}: не объект`), false;
    if (job.uses) continue; // reusable workflow
    if (!job['runs-on']) bad(file, `job ${id}: нет runs-on`);
    const steps = job.steps;
    if (!Array.isArray(steps) || !steps.length) {
      bad(file, `job ${id}: нет steps`);
      continue;
    }
    steps.forEach((st, i) => {
      const at = `job ${id}, шаг ${i + 1} (${st?.name || st?.uses || 'без имени'})`;
      if (!st || typeof st !== 'object') return bad(file, `${at}: не объект`);
      if (!st.uses && !st.run) return bad(file, `${at}: ни uses, ни run`);
      if (st.uses && st.run) warn(file, `${at}: и uses, и run — GitHub возьмёт только одно`);
      // пайп без pipefail: код возврата берётся от tee (эта грабля уже была в доме)
      if (typeof st.run === 'string' && st.run.includes('|') && /tee\b/.test(st.run)) {
        if (!/pipefail/.test(st.run) && !String(st.shell || '').includes('bash')) {
          bad(file, `${at}: пайп с tee без pipefail — падение теста даст зелёный шаг`);
        }
      }
    });
  }
  return true;
}

// --- 2. uses: существует ли экшен и тег -----------------------------------

function checkUses(file, doc) {
  if (offline) {
    notes.push('--offline: существование экшенов и тегов не проверялось');
    return;
  }
  const seen = new Map();
  for (const [id, job] of Object.entries(doc.jobs || {})) {
    const all = [...(job.steps || [])];
    if (job.uses) all.push({ uses: job.uses });
    for (const st of all) {
      const u = st?.uses;
      if (!u || u.startsWith('./') || u.startsWith('docker://')) continue;
      const [repoPath, ref] = u.split('@');
      const parts = repoPath.split('/');
      if (parts.length < 2) {
        bad(file, `job ${id}: «${u}» — не похоже на owner/repo@ref`);
        continue;
      }
      const repo = parts.slice(0, 2).join('/');
      if (!ref) {
        bad(file, `job ${id}: «${u}» без @ref — GitHub не примет`);
        continue;
      }
      if (!seen.has(`${repo}@${ref}`)) {
        seen.set(`${repo}@${ref}`, tagExists(repo, ref));
      }
      if (!seen.get(`${repo}@${ref}`)) {
        bad(file, `job ${id}: «${repo}@${ref}» — такого тега/ветки у экшена нет (или репозитория нет)`);
      }
    }
  }
}

// --- 3. покрытие ветки ----------------------------------------------------

function globToRe(glob) {
  // GitHub: `*` не переходит через `/`, `**` переходит
  const re = glob
    .replace(/[.+^${}()|[\]\\]/g, '\\$&')
    .replace(/\*\*/g, '\u0000')
    .replace(/\*/g, '[^/]*')
    .replace(/\u0000/g, '.*')
    .replace(/\?/g, '.');
  return new RegExp(`^${re}$`);
}

function checkBranchCoverage(file, doc, branch) {
  if (!branch) return;
  const on = doc.on ?? doc[true];
  if (!on) return;
  const events = typeof on === 'string' ? { [on]: null } : Array.isArray(on) ? Object.fromEntries(on.map((e) => [e, null])) : on;
  // Воркфлоу на issue_comment / workflow_dispatch / schedule к ветке не привязан —
  // спрашивать с него покрытие бессмысленно. Проверяем только тех, кто слушает
  // push или pull_request и при этом ограничил себя списком ветвей.
  const gated = ['push', 'pull_request'].filter((e) => e in events && events[e]?.branches);
  if (!gated.length) return;
  const covers = [];
  for (const ev of gated) {
    const cfg = events[ev];
    const patterns = cfg?.branches;
    if (!patterns) covers.push(`${ev}: все ветки`);
    else if (patterns.some((p) => globToRe(String(p)).test(branch))) covers.push(`${ev}: по маске`);
  }
  if (!covers.length) {
    bad(file, `не покрывает ветку «${branch}» — пуш в неё не запустит этот воркфлоу`);
  } else {
    notes.push(`${file}: покрывает «${branch}» (${covers.join(', ')})`);
  }
}

// --- 4. согласованность golangci-lint и конфига ---------------------------

function checkLintCompat(file, doc, cfg) {
  for (const [id, job] of Object.entries(doc.jobs || {})) {
    for (const st of job.steps || []) {
      if (!String(st?.uses || '').startsWith('golangci/golangci-lint-action')) continue;
      const major = Number(String(st.uses).split('@')[1].replace(/^v/, '').split('.')[0]);
      const lintVersion = String(st.with?.version || '');
      if (cfg && cfg.version === '2') {
        if (major < 7) {
          bad(file, `job ${id}: конфиг .golangci.yml версии 2, а экшен ${st.uses} — он не умеет v2 (нужен v7+)`);
        }
        if (lintVersion && !lintVersion.startsWith('v2') && lintVersion !== 'latest') {
          bad(file, `job ${id}: конфиг версии 2, а линтер указан «${lintVersion}» — нужен v2.x`);
        }
      }
      if (lintVersion === 'latest') {
        warn(file, `job ${id}: version: latest — версия проверки изменится без нас; лучше прибить`);
      }
    }
  }
}

// --- 5. .golangci.yml -----------------------------------------------------

function checkLintConfig(file) {
  if (!fs.existsSync(file)) return null;
  let cfg;
  try {
    cfg = yaml.load(fs.readFileSync(file, 'utf8'));
  } catch (e) {
    bad(path.basename(file), `YAML не читается: ${String(e.message).split('\n')[0]}`);
    return null;
  }
  if (!cfg || typeof cfg !== 'object') {
    bad(path.basename(file), 'пустой конфиг');
    return null;
  }
  const top = Object.keys(cfg);
  for (const k of top) {
    if (!V2_TOP.has(k)) bad(path.basename(file), `ключ «${k}» не из схемы v2 (v2: ${[...V2_TOP].join(', ')})`);
  }
  if (cfg.version === undefined) {
    bad(path.basename(file), 'нет `version: "2"` — golangci-lint v2 откажется читать конфиг');
  } else if (String(cfg.version) !== '2') {
    bad(path.basename(file), `version: ${JSON.stringify(cfg.version)} — поддерживается только "2"`);
  }
  const lint = cfg.linters || {};
  for (const k of Object.keys(lint)) {
    if (V1_ONLY.includes(k)) bad(path.basename(file), `linters.${k} — ключ схемы v1, в v2 это ${k === 'disable-all' ? '`default: none`' : k === 'enable-all' ? '`default: all`' : 'не существует'}`);
  }
  // run: в v2 нет skip-dirs/skip-files
  for (const k of Object.keys(cfg.run || {})) {
    if (V1_ONLY.includes(k)) bad(path.basename(file), `run.${k} — ключ схемы v1, в v2 его нет`);
  }
  // форматтеры: gofmt/goimports/gci/gofumpt живут в formatters, а не в linters.enable
  const fmts = ['gofmt', 'goimports', 'gci', 'gofumpt'];
  for (const f of fmts) {
    if ((lint.enable || []).includes(f)) {
      bad(path.basename(file), `линтер «${f}» в v2 — форматтер: перенести в formatters.enable`);
    }
  }
  for (const f of ['gosimple', 'stylecheck']) {
    if ((lint.enable || []).includes(f)) {
      bad(path.basename(file), `линтер «${f}» в v2 слит в staticcheck — отдельно его нет`);
    }
  }
  return cfg;
}

// --- сборка ---------------------------------------------------------------

function main() {
  const wfDir = path.join(root, '.github/workflows');
  const cfgPath = path.join(root, '.golangci.yml');
  const altCfg = ['.golangci.yaml', '.golangci.toml', '.golangci.json'].map((f) => path.join(root, f));
  const lintCfg = checkLintConfig(cfgPath) || (altCfg.some((p) => fs.existsSync(p)) ? (warn('.golangci.yml', 'конфиг линтера есть в другом формате — проверьте вручную'), null) : null);

  const files = fs.existsSync(wfDir)
    ? fs.readdirSync(wfDir).filter((f) => /\.ya?ml$/.test(f)).map((f) => path.join(wfDir, f))
    : [];

  console.log(`Корень: ${root}`);
  console.log(`Воркфлоу: ${files.length}; конфиг линтера: ${fs.existsSync(cfgPath) ? 'есть' : 'нет'}${branchArg ? `; ветка: ${branchArg}` : ''}\n`);

  if (!files.length) notes.push('каталога .github/workflows нет — проверять нечего');

  for (const file of files) {
    const rel = path.relative(root, file);
    let doc;
    try {
      doc = yaml.load(fs.readFileSync(file, 'utf8'));
    } catch (e) {
      bad(rel, `YAML не читается: ${String(e.message).split('\n')[0]}`);
      continue;
    }
    if (!checkStructure(rel, doc)) continue;
    checkUses(rel, doc);
    checkBranchCoverage(rel, doc, branchArg);
    checkLintCompat(rel, doc, lintCfg);
    console.log(`· ${rel} — jobs: ${Object.keys(doc.jobs).join(', ')}`);
  }

  if (notes.length) {
    console.log('\nЗаметки:');
    for (const n of notes) console.log(`  – ${n}`);
  }
  if (warnings.length) {
    console.log('\nПредупреждения:');
    for (const w of warnings) console.log(`  ! ${w}`);
  }
  if (problems.length) {
    console.log('\nПроблемы (GitHub выдаст ошибку или промолчит):');
    for (const p of problems) console.log(`  ✗ ${p}`);
  }
  console.log(`\nворкфлоу: ${files.length}, проблем: ${problems.length}, предупреждений: ${warnings.length}`);
  return problems.length ? 1 : 0;
}

process.exit(main());
