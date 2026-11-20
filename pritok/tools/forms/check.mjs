#!/usr/bin/env node
// Формы GitHub: сверка того, что объявлено в файлах, с тем, что есть в репозитории.
//
// Зачем. Форма — это файл, но работает она только если совпадает с настоящим
// репозиторием: метка из `labels:` должна существовать, имя файла в
// DISCUSSION_TEMPLATE должно совпадать со слагом категории, а в обсуждении
// должно быть хотя бы одно не-markdown поле. Когда что-то из этого разъезжается,
// GitHub не падает — он просто молча не показывает форму или не ставит метку.
// Молчание здесь дороже ошибки: кажется, что дверь есть, а её нет.
//
// Источники правил (прочитано 2026-11-19):
//   docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/configuring-issue-templates-for-your-repository
//   docs.github.com/en/discussions/managing-discussions-for-your-community/syntax-for-discussion-category-forms
//   docs.github.com/en/discussions/managing-discussions-for-your-community/creating-discussion-category-forms
//
// Что НЕ проверяется (граница честная): отрисовка формы в браузере, права на
// категории, поведение при отправке. Это проверка объявлений, а не прогон.
//
// Использование:
//   node check.mjs <корень-репозитория> [--labels labels.json] [--categories cats.json] [--offline]
//
// Без --offline берёт метки и категории из `gh` (нужен авторизованный gh).
// С --offline читает два JSON-файла, сохранённых заранее:
//   labels.json     — gh api repos/OWNER/REPO/labels --jq '[.[].name]'
//   categories.json — GraphQL discussionCategories → [{slug,name}]

import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import * as yaml from 'js-yaml';

const args = process.argv.slice(2);
const root = path.resolve(args[0] || '.');
const opt = (name) => {
  const i = args.indexOf(name);
  return i >= 0 ? args[i + 1] : null;
};
const offline = args.includes('--offline');

const FIELD_TYPES = new Set(['markdown', 'input', 'textarea', 'dropdown', 'checkboxes', 'upload']);

const problems = [];
const warnings = [];
const notes = [];
const bad = (file, msg) => problems.push(`${file}: ${msg}`);
const warn = (file, msg) => warnings.push(`${file}: ${msg}`);

// --- откуда берём настоящее состояние репозитория -------------------------

function gh(argsList) {
  return execFileSync('gh', argsList, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
}

function repoInfo() {
  if (offline) {
    const labelsFile = opt('--labels');
    const catsFile = opt('--categories');
    if (!labelsFile || !fs.existsSync(labelsFile)) {
      throw new Error('--offline требует --labels <файл> (список меток репозитория)');
    }
    if (!catsFile || !fs.existsSync(catsFile)) {
      throw new Error('--offline требует --categories <файл> (слаги категорий обсуждений)');
    }
    return {
      labels: new Set(JSON.parse(fs.readFileSync(labelsFile, 'utf8'))),
      categories: JSON.parse(fs.readFileSync(catsFile, 'utf8')).map((c) => c.slug),
      branches: null,
      source: 'файлы',
    };
  }
  const nwo = JSON.parse(gh(['repo', 'view', '--json', 'nameWithOwner'])).nameWithOwner;
  const labels = new Set(JSON.parse(gh(['api', `repos/${nwo}/labels?per_page=100`, '--jq', '[.[].name]'])));
  const catRaw = gh([
    'api', 'graphql', '-f',
    `query={repository(owner:"${nwo.split('/')[0]}",name:"${nwo.split('/')[1]}"){discussionCategories(first:25){nodes{slug name}}}}`,
  ]);
  const categories = JSON.parse(catRaw).data.repository.discussionCategories.nodes.map((c) => c.slug);
  const branches = JSON.parse(gh(['api', `repos/${nwo}/branches?per_page=100`, '--jq', '[.[].name]']));
  return { labels, categories, branches, source: `gh → ${nwo}` };
}

// --- общий проход по одной форме -----------------------------------------

function checkForm({ rel, doc, kind, slug, state }) {
  if (!doc || typeof doc !== 'object' || Array.isArray(doc)) {
    bad(rel, 'файл не читается как YAML-объект');
    return null;
  }

  const body = doc.body;
  if (!Array.isArray(body) || body.length === 0) {
    bad(rel, 'нет body — форма не примется');
    return null;
  }

  const nonMarkdown = body.filter((f) => f && f.type && f.type !== 'markdown');

  if (kind === 'discussion') {
    // Имя файла = слаг категории. Иначе форма молча не подключится.
    if (!state.categories.includes(slug) && state.categories.length) {
      bad(rel, `имя файла «${slug}» не совпадает ни с одним слагом категории (${state.categories.join(', ')}) — форма не подключится`);
    }
    // Минимум одно не-markdown поле — требование документации.
    if (nonMarkdown.length === 0) {
      bad(rel, 'только markdown-поля — GitHub требует хотя бы одно не-markdown, форма не примется');
    }
  } else {
    const name = typeof doc.name === 'string' ? doc.name : '';
    if (name.length <= 3) {
      bad(rel, `name «${name}» — 3 символа или меньше, форма не появится в списке`);
    }
  }

  // Поля: тип, id, подпись; у списков — варианты.
  const ids = new Set();
  body.forEach((f, i) => {
    const at = `поле #${i + 1}`;
    if (!f || typeof f !== 'object') return bad(rel, `${at}: не объект`);
    if (!f.type) return bad(rel, `${at}: нет type`);
    if (!FIELD_TYPES.has(f.type)) return bad(rel, `${at}: неизвестный type «${f.type}»`);
    if (f.type === 'markdown') {
      if (!f.attributes || !f.attributes.value) warn(rel, `${at} (markdown): пустой attributes.value`);
      return;
    }
    if (!f.id) bad(rel, `${at} (${f.type}): нет id — ответ не сохранится в теле`);
    else if (ids.has(f.id)) bad(rel, `${at}: id «${f.id}» повторяется`);
    else ids.add(f.id);

    if (!f.attributes || !f.attributes.label) bad(rel, `${at} (${f.id || '?'}): нет attributes.label`);
    const a = f.attributes || {};
    if (f.type === 'dropdown') {
      if (!Array.isArray(a.options) || a.options.length === 0) bad(rel, `${at} (${f.id}): dropdown без options`);
      else if (a.options.length > 0 && a.default !== undefined && !(a.default >= 0 && a.default < a.options.length)) {
        bad(rel, `${at} (${f.id}): default=${a.default} вне списка options`);
      }
    }
    if (f.type === 'checkboxes') {
      if (!Array.isArray(a.options) || a.options.length === 0) bad(rel, `${at} (${f.id}): checkboxes без options`);
    }
  });

  // Метки, которые форма ставит автоматически, должны существовать.
  const raw = doc.labels ?? [];
  const labels = Array.isArray(raw) ? raw : String(raw).split(',').map((s) => s.trim()).filter(Boolean);
  if (state.labels.size) {
    for (const l of labels) {
      if (!state.labels.has(l)) bad(rel, `ставит метку «${l}», а такой метки в репозитории нет — метка не применится`);
    }
  }

  return { rel, kind, slug, labels, fields: body.length, nonMarkdown: nonMarkdown.length };
}

// --- сборка --------------------------------------------------------------

function collect(dir, kind) {
  if (!fs.existsSync(dir)) return [];
  return fs
    .readdirSync(dir)
    .filter((f) => /\.ya?ml$/.test(f))
    .sort()
    .map((f) => ({ file: path.join(dir, f), slug: f.replace(/\.ya?ml$/, ''), kind }));
}

function main() {
  let state;
  try {
    state = repoInfo();
  } catch (e) {
    console.error(`не смогла узнать состояние репозитория: ${e.message}`);
    return 2;
  }

  const forms = [
    ...collect(path.join(root, '.github/ISSUE_TEMPLATE'), 'issue'),
    ...collect(path.join(root, '.github/DISCUSSION_TEMPLATE'), 'discussion'),
  ];

  if (!forms.length) {
    console.error(`в ${root} нет ни .github/ISSUE_TEMPLATE, ни .github/DISCUSSION_TEMPLATE`);
    return 2;
  }

  const ok = [];
  let config = null;

  for (const { file, slug, kind } of forms) {
    const rel = path.relative(root, file);

    let doc;
    try {
      doc = yaml.load(fs.readFileSync(file, 'utf8'));
    } catch (e) {
      bad(rel, `YAML не читается: ${String(e.message).split('\n')[0]}`);
      continue;
    }

    if (slug === 'config' && kind === 'issue') {
      config = { rel, doc: doc || {} };
      continue;
    }

    const res = checkForm({ rel, doc, kind, slug, state });
    if (res) ok.push(res);
  }

  // config.yml: ссылки на контактные ссылки и выключенные пустые задачи.
  if (config && config.doc) {
    const links = Array.isArray(config.doc.contact_links) ? config.doc.contact_links : [];
    for (const [i, link] of links.entries()) {
      const at = `contact_links #${i + 1}`;
      if (!link || !link.name || !link.url) bad(config.rel, `${at}: нужны name и url`);
      const url = String(link?.url || '');
      // Имя ветки может содержать «/» (arena/…), поэтому не режем по первому слэшу,
      // а сверяем с настоящим списком веток: он короткий и точный.
      if (url.includes('/blob/') && state.branches) {
        const branch = state.branches.find((b) => url.includes(`/blob/${b}/`));
        if (!branch) {
          bad(config.rel, `${at}: ссылка ведёт в ветку, которой нет (ветки: ${state.branches.join(', ')}) — ссылка мертва`);
        } else if (branch !== 'main') {
          warn(config.rel, `${at}: ведёт в ветку «${branch}», а не в основную — ссылка умрёт вместе с веткой`);
        }
      }
    }
    if (config.doc.blank_issues_enabled === true) {
      notes.push(`${config.rel}: свободные задачи включены — форма не обязательна, можно писать текстом`);
    }
  }

  // Порядок форм: GitHub сортирует по имени файла, префикс числа задаёт порядок.
  const issueNames = forms.filter((f) => f.kind === 'issue' && f.slug !== 'config').map((f) => f.slug);
  if (issueNames.some((n) => /^\d+-/.test(n))) {
    notes.push('порядок форм задан числовыми префиксами — 01…09, при 10+ файлах нужны двузначные');
  }

  // --- отчёт ---
  console.log(`Корень: ${root}`);
  console.log(`Состояние репозитория: ${state.source} (меток ${state.labels.size}, категорий ${state.categories.length})\n`);

  for (const f of ok) {
    console.log(`· ${f.rel}${f.kind === 'discussion' ? ` → категория ${f.slug}` : ''}  полей ${f.fields}, не-markdown ${f.nonMarkdown}`);
  }

  if (notes.length) {
    console.log('\nЗаметки:');
    for (const n of notes) console.log(`  – ${n}`);
  }
  if (warnings.length) {
    console.log('\nПредупреждения (форма работает, но хрупко):');
    for (const w of warnings) console.log(`  ! ${w}`);
  }
  if (problems.length) {
    console.log('\nПроблемы (GitHub промолчит — форма не покажется или метка не встанет):');
    for (const p of problems) console.log(`  ✗ ${p}`);
  }

  console.log(`\nформ: ${ok.length + (config ? 1 : 0)}, проблем: ${problems.length}, предупреждений: ${warnings.length}`);
  return problems.length ? 1 : 0;
}

process.exit(main());
