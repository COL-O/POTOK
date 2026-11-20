#!/usr/bin/env node
// docfact.mjs — сверка утверждений документов дома с фактом (git + GitHub API).
//
// Зачем: КОН 2 — факт, а не суждение. Документы дома умеют тихо расходиться
// с кодом: «PR на 233 файла» против 2834, «запуск пройден» против RUN-LOG.
// Скрипт не «верит» формулировкам, а проверяет то, что выразимо числом или
// ссылкой на файл, и печатает расхождения списком.
//
// Запуск:  node pritok/tools/docfact.mjs [--repo COL-O/POTOK] [--branch <ref>]
// Требует: git + gh (для чисел PR); без gh проверяются только локальные факты.

import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';

const args = process.argv.slice(2);
const opt = (name, def) => {
  const i = args.indexOf(name);
  return i >= 0 && args[i + 1] ? args[i + 1] : def;
};

const repo = opt('--repo', 'COL-O/POTOK');
const branch = opt('--branch', 'origin/arena/3bbbb51d-potok');
const root = path.resolve(import.meta.dirname, '../..');

const run = (cmd, argv) => {
  try {
    return execFileSync(cmd, argv, { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  } catch (e) {
    return { error: (e.stderr || e.message || '').toString().trim().split('\n').slice(-1)[0] };
  }
};

const houseFile = (p) => {
  const r = run('git', ['show', `${branch}:${p}`]);
  return typeof r === 'string' ? r : null;
};

const findings = [];
const ok = (what, detail) => findings.push({ level: 'OK', what, detail });
const bad = (what, doc, fact) => findings.push({ level: 'РАСХОЖДЕНИЕ', what, detail: `${doc}  ←→  факт: ${fact}` });
const skip = (what, why) => findings.push({ level: 'ПРОПУЩЕНО', what, detail: why });

// ---------- 1. Числа файлов в документах против git ----------
const tracked = run('git', ['ls-tree', '-r', '--name-only', branch]);
const fileCount = typeof tracked === 'string' ? tracked.split('\n').filter(Boolean).length : null;

if (fileCount) {
  ok('файлов в ветке дома', `${branch} → ${fileCount}`);
  ok('числа PR в документах', 'сверяются по API GitHub в блоке 4, не по дереву ветки');
}

// ---------- 2. Утверждения о запуске против RUN-LOG ----------
const runLog = houseFile('docs/ruslo-lab/RUN-LOG.md');
const roadmap = houseFile('docs/13-roadmap.md');
if (runLog && roadmap) {
  const runLogEmpty = /\| bootstrap \(корень\) \|\s*\|\s*\|/.test(runLog) || !/OK/.test(runLog);
  const roadmapClaimsDone = /Первый запуск пройден/i.test(roadmap);
  if (roadmapClaimsDone && runLogEmpty) {
    bad('состояние живого запуска',
      'docs/13-roadmap.md: «Первый запуск пройден»',
      'docs/ruslo-lab/RUN-LOG.md: журнал пуст, ни одной строки с OK');
  } else {
    ok('состояние живого запуска', 'документы согласованы');
  }
} else {
  skip('состояние живого запуска', 'нет одного из файлов');
}

// ---------- 3. Код, на который ссылаются документы: существует ли ----------
const codeRefs = [];
for (const p of ['docs/PROJECT-STATUS.md', 'community/SFERA_MEMORY.md', 'docs/13-roadmap.md']) {
  const text = houseFile(p) || '';
  for (const m of text.matchAll(/`((?:modules|core|cmd|contracts|links|deploy)\/[\w./-]+\.\w+)`/g)) {
    codeRefs.push({ doc: p, path: m[1] });
  }
}
const uniq = [...new Map(codeRefs.map((r) => [r.path, r])).values()];
for (const r of uniq) {
  const exists = run('git', ['cat-file', '-e', `${branch}:${r.path}`]);
  if (typeof exists === 'string' && exists.startsWith('fatal')) {
    bad('ссылка на файл', `${r.doc}: \`${r.path}\``, 'такого файла в ветке нет');
  }
}
if (uniq.length) ok('ссылки на файлы в документах', `проверено ${uniq.length}`);

// ---------- 4. Числа PR против GitHub API ----------
const prList = run('gh', ['pr', 'list', '--repo', repo, '--state', 'open', '--json', 'number,title,changedFiles,additions,deletions']);
if (typeof prList === 'string' && prList.startsWith('[')) {
  for (const pr of JSON.parse(prList)) {
    ok(`PR №${pr.number}`, `${pr.changedFiles} файлов, +${pr.additions}/−${pr.deletions} — ${pr.title}`);
    const memText = houseFile('community/SFERA_MEMORY.md') || '';
    const claim = memText.match(new RegExp(`PR\\s*№\\s*${pr.number}[^.]*?(\\d{2,5})\\s+файл`));
    if (claim && Number(claim[1]) !== pr.changedFiles) {
      bad(`PR №${pr.number}: число файлов`,
        `community/SFERA_MEMORY.md: «${claim[0]}»`,
        `по API GitHub: ${pr.changedFiles}`);
    }
  }
} else {
  skip('числа PR по API', 'gh недоступен или нет открытых PR');
}

// ---------- 5. CI: последний прогон ----------
const runs = run('gh', ['run', 'list', '--repo', repo, '--limit', '4', '--json', 'name,conclusion,headSha']);
if (typeof runs === 'string' && runs.startsWith('[')) {
  for (const r of JSON.parse(runs)) {
    ok(`CI ${r.name}`, `${r.conclusion} на ${r.headSha.slice(0, 7)}`);
  }
} else {
  skip('CI по API', 'gh недоступен');
}

// ---------- печать ----------
const pad = (s, n) => (s + ' '.repeat(n)).slice(0, n);
let diverged = 0;
for (const f of findings) {
  if (f.level === 'РАСХОЖДЕНИЕ') diverged++;
  console.log(`${pad(f.level, 14)} ${f.what}\n${' '.repeat(15)}${f.detail}`);
}
console.log(`\nВсего проверок: ${findings.length}, расхождений: ${diverged}`);
process.exit(diverged ? 1 : 0);
