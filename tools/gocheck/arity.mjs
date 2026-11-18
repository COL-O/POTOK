// Проверка числа аргументов у вызовов функций своего пакета (частая ошибка компиляции).
import fs from 'node:fs';
import path from 'node:path';
import { Parser, Language } from 'web-tree-sitter';

const root = path.resolve(process.argv[2] || '.');
await Parser.init();
const lang = await Language.load(path.join(process.cwd(), 'node_modules/tree-sitter-wasms/out/tree-sitter-go.wasm'));
const parser = new Parser(); parser.setLanguage(lang);

const files = [];
(function walk(d) {
  for (const e of fs.readdirSync(d, { withFileTypes: true })) {
    if (['.git','node_modules','.cache'].includes(e.name)) continue;
    const p = path.join(d, e.name);
    if (e.isDirectory()) walk(p); else if (e.name.endsWith('.go')) files.push(p);
  }
})(root);

const funcs = new Map(); // dir -> Map(name -> {min,max})
for (const f of files) {
  const rel = path.relative(root, f);
  const tree = parser.parse(fs.readFileSync(f, 'utf8'));
  const dir = path.dirname(rel);
  for (const c of tree.rootNode.children) {
    if (c.type !== 'function_declaration') continue;
    const nm = c.childForFieldName('name'); const params = c.childForFieldName('parameters');
    if (!nm || !params) continue;
    let n = 0, variadic = false;
    for (const p of params.children) {
      if (p.type === 'parameter_declaration') {
        const ids = p.children.filter((x) => x.type === 'identifier');
        n += ids.length || 1;
      }
      if (p.type === 'variadic_parameter_declaration') { variadic = true; }
    }
    if (!funcs.has(dir)) funcs.set(dir, new Map());
    const m = funcs.get(dir);
    const prev = m.get(nm.text);
    m.set(nm.text, { min: n, variadic: variadic || (prev && prev.variadic) });
  }
  tree.delete();
}

let problems = 0;
for (const f of files) {
  const rel = path.relative(root, f);
  const tree = parser.parse(fs.readFileSync(f, 'utf8'));
  const dir = path.dirname(rel);
  const m = funcs.get(dir);
  if (m) {
    (function w(n) {
      if (n.type === 'call_expression') {
        const fn = n.childForFieldName('function');
        const args = n.childForFieldName('arguments');
        if (fn && args && fn.type === 'identifier' && m.has(fn.text)) {
          const spec = m.get(fn.text);
          const cnt = args.children.filter((a) => a.type !== '(' && a.type !== ')' && a.type !== ',').length;
          if (spec.variadic ? cnt < spec.min - 1 : cnt !== spec.min) {
            problems++;
            console.log(`${rel}:${n.startPosition.row + 1} вызов ${fn.text}: аргументов ${cnt}, объявлено ${spec.min}${spec.variadic ? ' (variadic)' : ''}`);
          }
        }
      }
      for (const ch of n.children) w(ch);
    })(tree.rootNode);
  }
  tree.delete();
}
console.log(`\nПроблем с числом аргументов: ${problems}`);
