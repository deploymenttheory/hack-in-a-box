// Source inventory only. This does not certify native integration or release readiness.
import { readFileSync, existsSync } from 'node:fs'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { resolve, dirname } from 'node:path'
import { createHash } from 'node:crypto'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const require = createRequire(resolve(root, 'web/package.json'))
const ts = require('typescript')
const inventory = readFileSync(resolve(root, 'docs/design/portal-route-inventory.md'), 'utf8')
const routes = []
const errors = []
for (const [portal, path] of [['administrator', 'web/src/app.tsx'], ['participant', 'web/src/participant-app.tsx']]) {
  const text = readFileSync(resolve(root, path), 'utf8')
  const source = ts.createSourceFile(path, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
  const declared = new Map(), registered = new Set(), imports = new Map()
  function visit(node) {
    if (ts.isImportDeclaration(node) && node.importClause?.namedBindings && ts.isNamedImports(node.importClause.namedBindings)) {
      for (const binding of node.importClause.namedBindings.elements) imports.set(binding.name.text, node.moduleSpecifier.text)
    }
    if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer && ts.isCallExpression(node.initializer) && node.initializer.expression.getText(source) === 'createRoute') {
      const options = node.initializer.arguments[0]
      const properties = new Map(options.properties.filter(ts.isPropertyAssignment).map(p => [p.name.getText(source), p.initializer]))
      const route = properties.get('path'), component = properties.get('component')
      if (!route || !ts.isStringLiteral(route) || !component) errors.push(`Unresolved route declaration in ${path}: ${node.name.text}`)
      else declared.set(node.name.text, { path: route.text, component: component.getText(source) })
    }
    if (ts.isCallExpression(node) && node.expression.getText(source) === 'root.addChildren') {
      for (const item of node.arguments[0].elements) registered.add(item.getText(source))
    }
    ts.forEachChild(node, visit)
  }
  visit(source)
  for (const name of registered) {
    const route = declared.get(name)
    if (!route) { errors.push(`Unknown registered route ${name} in ${path}`); continue }
    const module = imports.get(route.component)
    const owner = module?.startsWith('.') ? resolve(dirname(resolve(root, path)), `${module}.tsx`) : undefined
    if (!owner || !existsSync(owner)) errors.push(`No concrete owner module for ${route.path} (${portal})`)
    if (!inventory.includes('`' + route.path + '`')) errors.push(`Registered route missing from migration inventory: ${route.path} (${portal})`)
    routes.push({ portal, ...route, owner: owner?.slice(root.length + 1), integration: 'not established by source inventory' })
  }
  for (const name of declared.keys()) if (!registered.has(name)) errors.push(`Declared route is not registered: ${name} in ${path}`)
}
const qualities = readFileSync(resolve(root, 'docs/design/fleet-reference/quality-coverage.csv'), 'utf8')
const ids = [...qualities.matchAll(/^Q(\d+),/gm)].map(match => Number(match[1]))
if (ids.length !== 100 || new Set(ids).size !== 100 || ids.some((id, index) => id !== index + 1)) errors.push('Reference applicability map must retain all 100 distinct ordered quality identities')
const sourceFiles = ['web/src/app.tsx', 'web/src/participant-app.tsx', 'web/src/main.tsx', 'docs/design/portal-route-inventory.md']
const result = { kind: 'source coverage; not browser/native API proof', sources: sourceFiles.map(path => ({ path, sha256: createHash('sha256').update(readFileSync(resolve(root, path))).digest('hex') })), routes, independentEntries: [{ path: '/display/$eventId', owner: 'web/src/room-display.tsx', portal: 'participant' }, { path: '/', owner: 'internal/styleguide/server.go', portal: 'independent static guide; verify its host separately' }], referenceQualities: ids.length, errors }
console.log(JSON.stringify(result, null, 2))
if (errors.length) process.exitCode = 1
