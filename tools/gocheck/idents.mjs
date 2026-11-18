// Эвристика: идентификатор с большой буквы, использованный без квалификатора,
// но не объявленный ни в файле, ни в пакете, ни как поле/локальная переменная —
// кандидат на «undefined» при компиляции.
import fs from 'node:fs';
import path from 'node:path';
import { Parser, Language } from 'web-tree-sitter';

const root = path.resolve(process.argv[2] || '.');
await Parser.init();
const lang = await Language.load(path.join(process.env.GOCHECK_HOME || process.cwd(), 'node_modules/tree-sitter-wasms/out/tree-sitter-go.wasm'));
const parser = new Parser(); parser.setLanguage(lang);

const BUILTIN = new Set(['true','false','nil','iota','string','int','int64','uint16','uint32','uint64','float64','byte','rune','bool','error','any','append','len','cap','make','new','copy','delete','min','max','panic','recover','print','println','T','B','M','F','Fatal','Errorf','Fatalf','Logf','Sprintf','Printf','Println','Error','New','Value','Kind','Read','Write','Open','Close']);

const files = [];
(function walk(d) {
  for (const e of fs.readdirSync(d, { withFileTypes: true })) {
    if (['.git','node_modules','.cache'].includes(e.name)) continue;
    const p = path.join(d, e.name);
    if (e.isDirectory()) walk(p); else if (e.name.endsWith('.go')) files.push(p);
  }
})(root);

const packages = new Map();
for (const f of files) {
  const rel = path.relative(root, f);
  const tree = parser.parse(fs.readFileSync(f, 'utf8'));
  const dir = path.dirname(rel);
  const key = dir;
  const rec = packages.get(key) || { declared: new Set(), uses: [] };
  (function w(n) {
    if (['type_spec','const_spec','var_spec','function_declaration','method_declaration','field_identifier','parameter_declaration','short_var_declaration','assignment_statement','label_name','dot_import'].includes(n.type)) {
      for (const ch of n.children) if (ch.type === 'identifier' || ch.type === 'field_identifier' || ch.type === 'type_identifier') rec.declared.add(ch.text);
      const nm = n.childForFieldName && n.childForFieldName('name');
      if (nm) rec.declared.add(nm.text);
    }
    const OKPARENT = new Set(['argument_list','binary_expression','assignment_statement','return_statement','call_expression','unary_expression','index_expression','expression_list','parenthesized_expression','short_var_declaration','composite_literal','literal_value','slice_type','map_type','array_type','variadic_argument','inc_statement','type_assertion_expression']);
    if (n.type === 'identifier' && /^[A-Z]/.test(n.text) && OKPARENT.has(n.parent && n.parent.type)) {
      const parent = n.parent;
      const ok = parent && ['selector_expression','qualified_type'].includes(parent.type) && parent.childForFieldName('operand') === n || (parent && parent.type === 'qualified_type' && parent.childForFieldName('package') === n);
      if (!ok) rec.uses.push({ rel, line: n.startPosition.row + 1, name: n.text, parent: parent && parent.type });
    }
    for (const ch of n.children) w(ch);
  })(tree.rootNode);
  packages.set(key, rec);
  tree.delete();
}

let problems = 0;
for (const [dir, rec] of packages) {
  const unknown = rec.uses.filter((u) => !rec.declared.has(u.name) && !BUILTIN.has(u.name));
  if (unknown.length) {
    problems += unknown.length;
    const uniq = [...new Map(unknown.map((u) => [u.name + '@' + u.rel + ':' + u.line, u])).values()];
    console.log(`# ${dir || '.'}`);
    for (const u of uniq.slice(0, 12)) console.log(`  ${u.rel}:${u.line} ${u.name} (${u.parent})`);
  }
}
console.log(`\nНеизвестных имён: ${problems}`);
process.exit(problems ? 1 : 0);
