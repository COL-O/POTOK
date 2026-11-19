// Синтаксическая проверка Go-файлов через tree-sitter-wasm (замена `go vet`).
import fs from 'node:fs';
import path from 'node:path';
import { Parser, Language } from 'web-tree-sitter';

const root = process.argv[2] || '.';
const wasm = path.join(process.env.GOCHECK_HOME || process.cwd(), 'node_modules/tree-sitter-wasms/out/tree-sitter-go.wasm');

await Parser.init();
const lang = await Language.load(wasm);
const parser = new Parser();
parser.setLanguage(lang);

const files = [];
(function walk(dir) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    if (e.name === '.git' || e.name === 'node_modules' || e.name === '.cache') continue;
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p);
    else if (e.name.endsWith('.go')) files.push(p);
  }
})(root);

let bad = 0;
const problems = [];
for (const f of files.sort()) {
  const src = fs.readFileSync(f, 'utf8');
  const tree = parser.parse(src);
  if (tree.rootNode.hasError) {
    bad++;
    (function collect(n) {
      if (n.type === 'ERROR' || n.isMissing) {
        problems.push(`FAIL ${path.relative(root, f)}:${n.startPosition.row + 1} ${n.type}${n.isMissing ? ' (missing)' : ''} "${n.text.slice(0, 70).replace(/\n/g, '\\n')}"`);
      }
      for (const c of n.children) collect(c);
    })(tree.rootNode);
  }
  tree.delete();
}
if (problems.length) console.log(problems.slice(0, 20).join('\n'));
console.log(`Проверено файлов: ${files.length}, с ошибками разбора: ${bad}`);
process.exit(bad ? 1 : 0);
