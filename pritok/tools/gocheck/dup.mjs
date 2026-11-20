// Поиск дублирующихся объявлений на уровне пакета (ошибка компиляции Go).
import fs from 'node:fs';
import path from 'node:path';
import { Parser, Language } from 'web-tree-sitter';

const root = path.resolve(process.argv[2] || '.');
await Parser.init();
const lang = await Language.load(path.join(process.env.GOCHECK_HOME || process.cwd(), 'node_modules/tree-sitter-wasms/out/tree-sitter-go.wasm'));
const parser = new Parser(); parser.setLanguage(lang);

const files = [];
(function walk(d) {
  for (const e of fs.readdirSync(d, { withFileTypes: true })) {
    if (['.git','node_modules','.cache'].includes(e.name)) continue;
    const p = path.join(d, e.name);
    if (e.isDirectory()) walk(p); else if (e.name.endsWith('.go')) files.push(p);
  }
})(root);

const byPkg = new Map();
for (const f of files) {
  const rel = path.relative(root, f);
  const tree = parser.parse(fs.readFileSync(f, 'utf8'));
  const dir = path.dirname(rel);
  const isTest = rel.endsWith('_test.go');
  const decls = [];
  for (const c of tree.rootNode.children) {
    const decl = (n, kind) => {
      const nm = n.childForFieldName && n.childForFieldName('name');
      if (nm) decls.push({ name: nm.text, kind });
    };
    if (c.type === 'function_declaration') decl(c, 'func');
    if (c.type === 'type_declaration') for (const s of c.children) if (s.type === 'type_spec') decl(s, 'type');
    if (c.type === 'const_declaration' || c.type === 'var_declaration')
      for (const s of c.children) if (s.type === 'const_spec' || s.type === 'var_spec') decl(s, c.type);
  }
  const key = dir + (isTest ? '|test' : '');
  if (!byPkg.has(key)) byPkg.set(key, []);
  byPkg.get(key).push({ rel, decls, isTest, dir });
  tree.delete();
}

// тестовые файлы одного пакета компилируются вместе с основными
const merged = new Map();
for (const [, list] of byPkg) for (const f of list) {
  const k = f.dir;
  if (!merged.has(k)) merged.set(k, []);
  merged.get(k).push(f);
}
let problems = 0;
for (const [dir, list] of merged) {
  const seen = new Map();
  for (const f of list) for (const d of f.decls) {
    if (d.kind === 'method') continue;
    if (d.name === '_') continue; // пустой идентификатор можно объявлять многократно
    const prev = seen.get(d.name);
    if (prev) { console.log(`ДУБЛЬ ${dir || '.'}: ${d.name} — ${prev} и ${f.rel}`); problems++; }
    else seen.set(d.name, f.rel);
  }
}
console.log(`\nДублей объявлений: ${problems}`);
process.exit(problems ? 1 : 0);
