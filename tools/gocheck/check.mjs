// Лёгкая проверка Go-пакетов без тулчейна:
//  1) разбор через tree-sitter (ошибки синтаксиса);
//  2) обращения «пакет.Имя» → имя должно быть объявлено в целевом пакете;
//  3) используемый, но не импортированный пакет;
//  4) импортированный, но неиспользуемый пакет.
import fs from 'node:fs';
import path from 'node:path';
import { Parser, Language } from 'web-tree-sitter';

const root = path.resolve(process.argv[2] || '.');
const modName = fs.readFileSync(path.join(root, 'go.mod'), 'utf8').match(/^module\s+(\S+)/m)[1];
const STD = new Set(['context','fmt','errors','sort','strings','sync','time','os','io','json','testing','log','math','strconv','regexp','bytes','bufio','http','filepath','crypto','sha256','hex','encoding','net','reflect','runtime','unicode','path','flag','atomic','rand','signal']);

await Parser.init();
const lang = await Language.load(path.join(process.env.GOCHECK_HOME || process.cwd(), 'node_modules/tree-sitter-wasms/out/tree-sitter-go.wasm'));
const parser = new Parser();
parser.setLanguage(lang);

const files = [];
(function walk(dir) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    if (['.git', 'node_modules', '.cache'].includes(e.name)) continue;
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p); else if (e.name.endsWith('.go')) files.push(p);
  }
})(root);

const pkg = new Map(); // import path -> {name, exported:Set}
const structInfo = new Map(); // import path -> Map(typeName -> Set(field))
const perFile = [];

for (const f of files) {
  const rel = path.relative(root, f);
  const tree = parser.parse(fs.readFileSync(f, 'utf8'));
  if (tree.rootNode.hasError) console.log(`СИНТАКСИС ${rel}`);
  const dir = path.dirname(rel);
  const imp = dir === '.' ? modName : modName + '/' + dir.split(path.sep).join('/');
  const pkgClause = tree.rootNode.children.find((c) => c.type === 'package_clause');
  const nameNode = pkgClause && pkgClause.children.find((c) => c.type === 'package_identifier');
  const rec = pkg.get(imp) || { name: nameNode && nameNode.text, exported: new Set() };
  if (nameNode) rec.name = nameNode.text;
  pkg.set(imp, rec);

  const imports = new Map();
  const usedAliases = new Set();
  // Локальные имена (в т.ч. переменные, названные как пакет: store, stratos),
  // объявленные в файле через := или var — это не обращения к пакету.
  const localNames = new Set();
  (function collectLocals(n) {
    if (n.type === 'short_var_declaration') {
      const left = n.childForFieldName('left');
      for (const id of (left ? left.children : [])) {
        if (id.type === 'identifier') localNames.add(id.text);
      }
    }
    if (n.type === 'var_spec') {
      const nm = n.childForFieldName('name');
      if (nm) localNames.add(nm.text);
      for (const id of n.children) if (id.type === 'identifier') localNames.add(id.text);
    }
    if (n.type === 'parameter_declaration') {
      for (const id of n.children) if (id.type === 'identifier') localNames.add(id.text);
    }
    if (n.type === 'assignment_statement') {
      const left = n.childForFieldName('left');
      for (const id of (left ? left.children : [])) if (id.type === 'identifier') localNames.add(id.text);
    }
    for (const ch of n.children) collectLocals(ch);
  })(tree.rootNode);
  for (const c of tree.rootNode.children) {
    if (c.type !== 'import_declaration') continue;
    const specs = [];
    for (const s of c.children) {
      if (s.type === 'import_spec') specs.push(s);
      if (s.type === 'import_spec_list') for (const x of s.children) if (x.type === 'import_spec') specs.push(x);
    }
    for (const s of specs) {
      const p = s.childForFieldName('path'); if (!p) continue;
      const ip = p.text.replace(/^"|"$/g, '');
      const aliasNode = s.children.find((x) => ['package_identifier','blank_identifier','dot'].includes(x.type));
      const alias = aliasNode ? aliasNode.text : ip.split('/').pop();
      imports.set(alias, { ip, line: s.startPosition.row + 1, blank: alias === '_' || alias === '.' });
    }
  }

  const structs = new Map(); // typeName -> Set(field)
  (function walkStruct(n) {
    if (n.type === 'type_declaration') {
      for (const spec of n.children) {
        if (spec.type !== 'type_spec') continue;
        const tn = spec.childForFieldName('name');
        const ty = spec.childForFieldName('type');
        if (!tn || !ty || ty.type !== 'struct_type') continue;
        const set = new Set();
        for (const fl of ty.children) {
          if (fl.type !== 'field_declaration_list') continue;
          for (const f of fl.children) {
            if (f.type !== 'field_declaration') continue;
            for (const id of f.children) {
              if (id.type === 'field_identifier') set.add(id.text);
              if (id.type === 'field_declaration' ) break;
            }
          }
        }
        // встроенные поля (Meta, Embedded) — тоже поля структуры
        for (const fl of ty.children) {
          if (fl.type !== 'field_declaration_list') continue;
          for (const f of fl.children) {
            if (f.type !== 'field_declaration') continue;
            for (const id of f.children) {
              if (['type_identifier', 'qualified_type', 'generic_type'].includes(id.type)) {
                const name = id.text.includes('.') ? id.text.split('.').pop() : id.text;
                set.add(name);
              }
            }
          }
        }
        structs.set(tn.text, set);
      }
    }
    for (const ch of n.children) walkStruct(ch);
  })(tree.rootNode);
  structInfo.set(imp, structs);

  const decls = [];
  (function walkDecls(n) {
    if (['function_declaration','method_declaration','type_spec','const_spec','var_spec','type_declaration','const_declaration','var_declaration'].includes(n.type)) {
      const nm = n.childForFieldName('name');
      if (nm && n.type !== 'type_declaration' && n.type !== 'const_declaration' && n.type !== 'var_declaration') rec.exported.add(nm.text);
      if (!nm && ['type_declaration','const_declaration','var_declaration'].includes(n.type)) {
        for (const ch of n.children) {
          const n2 = ch.childForFieldName && ch.childForFieldName('name');
          if (n2) rec.exported.add(n2.text);
        }
      }
    }
    for (const ch of n.children) walkDecls(ch);
  })(tree.rootNode);

  const problems = [];
  (function walkSel(n) {
    if (n.type === 'qualified_type') {
      const op = n.childForFieldName('package');
      const fld = n.childForFieldName('name');
      if (op && fld) {
        usedAliases.add(op.text);
        const im = imports.get(op.text);
        const shadowed = !im && (localNames.has(op.text) || ![...pkg.values()].some((x) => x.name === op.text));
        if (im && im.ip.includes('.')) {
          const p = pkg.get(im.ip);
          if (p && p.exported.size && !p.exported.has(fld.text))
            problems.push(`${rel}:${n.startPosition.row+1} ${op.text}.${fld.text} — нет такого имени в ${im.ip}`);
        } else if (!im && !shadowed && [...pkg.values()].some((x) => x.name === op.text)) {
          problems.push(`${rel}:${n.startPosition.row+1} ${op.text}.${fld.text} — пакет «${op.text}» не импортирован`);
        }
      }
    }
    if (n.type === 'selector_expression') {
      const op = n.childForFieldName('operand');
      const fld = n.childForFieldName('field');
      if (op && fld && op.type === 'identifier') {
        usedAliases.add(op.text);
        const im = imports.get(op.text);
        if (im && im.ip.includes('.')) {
          const p = pkg.get(im.ip);
          if (p && p.exported.size && !p.exported.has(fld.text))
            problems.push(`${rel}:${n.startPosition.row+1} ${op.text}.${fld.text} — нет такого имени в ${im.ip}`);
        } else if (!im && !localNames.has(op.text)) {
          // идентификатор похож на пакет репозитория/стандартной библиотеки, но не импортирован
          if ([...pkg.values()].some((x) => x.name === op.text) || STD.has(op.text))
            problems.push(`${rel}:${n.startPosition.row+1} ${op.text}.${fld.text} — пакет «${op.text}» не импортирован`);
        }
      }
    }
    for (const ch of n.children) walkSel(ch);
  })(tree.rootNode);

  for (const [alias, im] of imports) {
    if (im.blank) continue;
    if (!usedAliases.has(alias)) problems.push(`${rel}:${im.line} импорт «${alias}» (${im.ip}) не используется`);
  }
  tree.delete();
  if (problems.length) console.log(problems.join('\n'));
}

// второй проход: поля в composite literal вида pkg.Type{Field: ...}
const fieldProblems = [];
for (const f of files) {
  const rel = path.relative(root, f);
  const tree = parser.parse(fs.readFileSync(f, 'utf8'));
  const imports = new Map();
  for (const c of tree.rootNode.children) {
    if (c.type !== 'import_declaration') continue;
    const specs = [];
    for (const s of c.children) {
      if (s.type === 'import_spec') specs.push(s);
      if (s.type === 'import_spec_list') for (const x of s.children) if (x.type === 'import_spec') specs.push(x);
    }
    for (const s of specs) {
      const p = s.childForFieldName('path'); if (!p) continue;
      const ip = p.text.replace(/^"|"$/g, '');
      const aliasNode = s.children.find((x) => ['package_identifier','blank_identifier','dot'].includes(x.type));
      imports.set(aliasNode ? aliasNode.text : ip.split('/').pop(), ip);
    }
  }
  (function w(n) {
    if (n.type === 'composite_literal') {
      const ty = n.childForFieldName('type');
      const body = n.childForFieldName('body');
      if (ty && body && ty.type === 'qualified_type') {
        const pkgId = ty.childForFieldName('package').text;
        const typeName = ty.childForFieldName('name').text;
        const ip = imports.get(pkgId);
        const st = ip && structInfo.get(ip);
        const fields = st && st.get(typeName);
        if (fields) {
          for (const el of body.children) {
            if (el.type !== 'keyed_element') continue;
            const k = el.children[0];
            if (k && k.type === 'literal_element' && k.children[0] && k.children[0].type === 'identifier') {
              const fname = k.children[0].text;
              if (!fields.has(fname))
                fieldProblems.push(`${rel}:${el.startPosition.row + 1} ${typeName}{${fname}: ...} — нет такого поля`);
            }
          }
        }
      }
    }
    for (const ch of n.children) w(ch);
  })(tree.rootNode);
  tree.delete();
}
if (fieldProblems.length) console.log(fieldProblems.join('\n'));
console.log(`\nФайлов: ${files.length}, пакетов: ${pkg.size}, проблем с полями: ${fieldProblems.length}`);
