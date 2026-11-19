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
const enumInfo = new Map();   // import path -> Map(constName -> {typeName, kind})
const fieldMeta = new Map();  // import path -> Map(typeName -> Map(field -> {typeName, pkg}))
const typeBodyByPkg = new Map(); // import path -> Map(typeName -> 'number'|'string'|'bool'|'other')

// вид значения: число, строка, bool или «ссылка на константу» (pkg.Name / Name)
function valueKindOf(node) {
  if (!node) return { kind: 'other' };
  let n = node;
  // literal_element -> expression_list -> сам литерал/ссылка
  while (n && ['literal_element', 'expression_list'].includes(n.type)) n = n.children[0];
  if (!n) return { kind: 'other' };
  if (['int_literal', 'float_literal', 'rune_literal'].includes(n.type)) return { kind: 'number' };
  if (['interpreted_string_literal', 'raw_string_literal'].includes(n.type)) return { kind: 'string' };
  if (n.type === 'true' || n.type === 'false') return { kind: 'bool' };
  // у selector_expression в грамматике Go поле называется field; берём текст целиком
  const name = n.type === 'identifier' ? n.text
    : n.type === 'selector_expression' ? n.text.split('.').pop()
    : null;
  return { kind: 'ref', constName: name };
}
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
        const types = new Map();
        for (const fl of ty.children) {
          if (fl.type !== 'field_declaration_list') continue;
          for (const f of fl.children) {
            if (f.type !== 'field_declaration') continue;
            const fname = f.children.find((x) => x.type === 'field_identifier');
            const ftype = f.childForFieldName ? f.childForFieldName('type') : null;
            for (const id of f.children) {
              if (id.type === 'field_identifier') set.add(id.text);
            }
            if (fname && ftype) {
              const typeKind = ftype.type === 'qualified_type'
                ? { typeName: ftype.childForFieldName('name').text, pkg: ftype.childForFieldName('package').text }
                : { typeName: ftype.text, pkg: null };
              types.set(fname.text, typeKind);
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
        if (!fieldMeta.has(imp)) fieldMeta.set(imp, new Map());
        fieldMeta.get(imp).set(tn.text, types);
      }
    }
    for (const ch of n.children) walkStruct(ch);
  })(tree.rootNode);
  structInfo.set(imp, structs);

  // тела типов: type K <body> — чтобы знать, объявлен ли тип от int или от string
  const typeBodies = new Map();
  (function walkTypeBodies(n) {
    if (n.type === 'type_spec') {
      const tn = n.childForFieldName('name');
      const tb = n.childForFieldName('type');
      if (tn && tb) typeBodies.set(tn.text, ['int', 'int8', 'int16', 'int32', 'int64', 'uint8', 'uintptr'].includes(tb.text) ? 'number'
        : tb.text === 'string' ? 'string' : tb.text === 'bool' ? 'bool' : 'other');
    }
    for (const ch of n.children) walkTypeBodies(ch);
  })(tree.rootNode);

  if (!typeBodyByPkg.has(imp)) typeBodyByPkg.set(imp, new Map());
  for (const [k, v] of typeBodies) typeBodyByPkg.get(imp).set(k, v);

  // тела перечислений: const X MyType = <literal> — запоминаем вид литерала
  for (const c of tree.rootNode.children) {
    if (c.type !== 'const_declaration') continue;
    for (const spec of c.children) {
      if (spec.type !== 'const_spec') continue;
      const ids = spec.children.filter((x) => x.type === 'identifier');
      const ty = spec.childForFieldName ? spec.childForFieldName('type') : null;
      const valWrap = spec.childForFieldName ? spec.childForFieldName('value') : null;
      // value в грамматике — expression_list; литерал лежит внутри него
      const val = valWrap && valWrap.type === 'expression_list' ? valWrap.children[0] : valWrap;
      if (!ids.length || !ty) continue;
      // литерала может не быть (строка перечисления без значения) — тогда тело
      // берётся у самого типа, иначе проверка «молчит» на реальных перечислениях
      // iota — числовое тело; в грамматике это identifier, а не литерал,
      // поэтому без отдельной ветки реальные перечисления дома (uint8 + iota)
      // давали kind='other', и проверка молчала (найдено 2026-11-18).
      const kind = !val ? (typeBodies.get(ty.text) || 'other')
        : val.type === 'identifier' && val.text === 'iota' ? 'number'
        : ['int_literal', 'float_literal', 'rune_literal'].includes(val.type) ? 'number'
        : ['interpreted_string_literal', 'raw_string_literal'].includes(val.type) ? 'string'
        : val.type === 'true' || val.type === 'false' ? 'bool' : 'other';
      for (const id of ids) {
        if (!enumInfo.has(imp)) enumInfo.set(imp, new Map());
        enumInfo.get(imp).set(id.text, { typeName: ty.text, kind });
      }
    }
  }

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

// проход 2b: константы перечисления — тело должно соответствовать типу
const enumProblems = [];
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
      const aliasNode = s.children.find((x) => ['package_identifier', 'blank_identifier', 'dot'].includes(x.type));
      imports.set(aliasNode ? aliasNode.text : ip.split('/').pop(), ip);
    }
  }
  (function w(n) {
    if (n.type === 'composite_literal') {
      const ty = n.childForFieldName('type');
      const body = n.childForFieldName('body');
      if (ty && body && ['qualified_type', 'identifier', 'type_identifier'].includes(ty.type)) {
        // тип может быть чужим (contracts.RollbackEvent) или своим (S)
        const dir = path.dirname(path.relative(root, f));
        const selfPkg = dir === '.' ? modName : modName + '/' + dir.split(path.sep).join('/');
        const pkgId = ty.type === 'qualified_type' ? ty.childForFieldName('package').text : null;
        const ip = pkgId ? imports.get(pkgId) : selfPkg;
        const typeName = ty.type === 'qualified_type' ? ty.childForFieldName('name').text
          : ty.type === 'type_identifier' ? ty.text : null;
        const fieldTypes = (typeName && fieldMeta.get(ip) && fieldMeta.get(ip).get(typeName)) || new Map();
        // перечисление может принадлежать не пакету литерала, а пакету типа поля
        // (S{Kind: c.Rollback300}: S — свой, Kind — contracts.RollbackKind):
        // поэтому здесь НЕЛЬЗЯ выходить, если в своём пакете перечислений нет.
        const enums = (ip && enumInfo.get(ip)) || null;
        // только СВОИ поля литерала: вложенный contracts.Meta{Source: ...}
        // относится к другому типу, и его поля нельзя мерить полями внешнего
        // (ложное срабатывание на projector.go:42, найдено 2026-11-18)
        const keyed = body.children.filter((x) => x.type === 'keyed_element');

        for (const el of keyed) {
          const k = el.children[0];
          const v = el.children.find((x) => x.type === 'literal_element' && x !== k);
          if (!k || !v || k.children[0]?.type !== 'identifier') continue;
          const fieldName = k.children[0].text;
          // 1) поле-перечисление с тем же именем, что константа (Suggestion.Source)
          let info = enums ? enums.get(fieldName) : null;
          // 2) поле объявлено типом-перечислением: contracts.RollbackEvent{Kind: contracts.Rollback300}
          if (!info) {
            const ft = fieldTypes.get(fieldName);
                    if (ft) {
              const owner = ft.pkg ? imports.get(ft.pkg) : (ip || null);
              const ownerEnums = owner && enumInfo.get(owner);
              // ищем константу по имени значения и сверяем её тип с типом поля
              const valName = v.text.includes('.') ? v.text.split('.').pop() : v.text;
              if (ownerEnums && ownerEnums.has(valName)) info = { ...ownerEnums.get(valName), expect: ft.typeName };
            }
          }
          // 3) поле объявлено типом с известным телом, а значение — литерал:
          //     RollbackEvent{Kind: "300_repair"} при RollbackKind uint8
          if (!info) {
            const ft = fieldTypes.get(fieldName);
                  const val = valueKindOf(v);
            if (ft && (val.kind === 'number' || val.kind === 'string')) {
              const owner = ft.pkg ? imports.get(ft.pkg) : ip;
              const bodies = owner && typeBodyByPkg.get(owner);
              const body = bodies && bodies.get(ft.typeName);
              if (body && body !== 'other' && body !== val.kind) {
                enumProblems.push(`${rel}:${el.startPosition.row + 1} ${fieldName} = ${v.text.slice(0, 24)} (${val.kind}) — поле объявлено ${ft.typeName} с телом ${body}`);
              }
            }
          }
          if (!info) continue;
          const valNode = v.type === 'expression_list' ? v.children[0]
            : v.children && v.children[0] ? v.children[0] : v;
          let val = valueKindOf(v);
          // значение-ссылка на константу (Kind: contracts.Rollback300) — берём её тело
          if (val.kind === 'ref') {
            // ссылка может указывать в другой пакет (bus.LayerFront) — тогда
            // тела не знаем и молчим: «не проверено» честнее выдуманной находки
            // имя константы может лежать в любом пакете проекта — ищем по всем
            let refOwner = (enums && enums.get(val.constName)) || null;
            if (!refOwner) {
              for (const m of enumInfo.values()) {
                if (m.has(val.constName)) { refOwner = m.get(val.constName); break; }
              }
            }
            if (!refOwner) continue;
            val = { kind: refOwner.kind };
          }
          const valKind = val.kind;
          // с чем сверяем: с телом ОБЪЯВЛЕННОГО типа поля (RollbackKind int),
          // а не с телом самой константы — иначе проверка тавтологична
          const ft = fieldTypes.get(fieldName);
          let declaredKind = info.kind;
          if (ft) {
            const ownerPkg = ft.pkg ? imports.get(ft.pkg) : ip;
            const bodies = ownerPkg && typeBodyByPkg.get(ownerPkg);
            const b = bodies && bodies.get(ft.typeName);
            if (b && b !== 'other') declaredKind = b;
          }
          info = { ...info, kind: declaredKind };
          if (valKind !== 'other' && info.kind !== 'other' && valKind !== info.kind) {
            const expect = info.expect ? ` (поле объявлено ${info.expect})` : '';
            enumProblems.push(`${rel}:${el.startPosition.row + 1} ${fieldName} = ${valNode.text.slice(0, 24)} (${valKind}) — тип ${info.typeName} объявлен с телом ${info.kind}${expect}`);
          }
        }
      }
    }
    for (const ch of n.children) w(ch);
  })(tree.rootNode);
  tree.delete();
}
if (enumProblems.length) console.log(enumProblems.join('\n'));

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
console.log(`\nФайлов: ${files.length}, пакетов: ${pkg.size}, проблем с полями: ${fieldProblems.length}, с перечислениями: ${enumProblems.length}`);
// без ненулевого кода проверка бесполезна в CI: «нашёл» и «чисто» выглядели одинаково
process.exit(fieldProblems.length + enumProblems.length ? 1 : 0);
