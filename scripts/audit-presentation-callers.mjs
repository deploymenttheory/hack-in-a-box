// Conservative source reachability; this does not establish rendered or API parity.
import { readFileSync, readdirSync, existsSync, statSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, resolve, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const require = createRequire(resolve(root, 'web/package.json'))
const ts = require('typescript')
const base = resolve(root, 'web/src')
const audit = JSON.parse(readFileSync(resolve(root, 'docs/verification/fleet-cutover/presentation-caller-audit.json'), 'utf8'))
const reachable = new Set(), errors = []
function visit(file) {
  if (reachable.has(file)) return
  reachable.add(file)
  const text = readFileSync(file, 'utf8'), dependencies = []
  if (file.endsWith('.css')) {
    dependencies.push(...[...text.matchAll(/@import\s+["']([^"']+)["']/g)].map(match => match[1]))
  } else {
    const source = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true)
    function collect(node) {
      if ((ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) && node.moduleSpecifier && ts.isStringLiteral(node.moduleSpecifier)) dependencies.push(node.moduleSpecifier.text)
      if (ts.isCallExpression(node) && node.expression.kind === ts.SyntaxKind.ImportKeyword) {
        if (node.arguments[0] && ts.isStringLiteral(node.arguments[0])) dependencies.push(node.arguments[0].text)
        else errors.push(`Review nonliteral dynamic import: ${relative(root, file)}`)
      }
      ts.forEachChild(node, collect)
    }
    collect(source)
  }
  for (const dependency of dependencies) {
    if (!dependency.startsWith('.') && !dependency.startsWith('@/')) continue
    const target = dependency.startsWith('@/') ? resolve(base, dependency.slice(2)) : resolve(dirname(file), dependency)
    const found = ['', '.tsx', '.ts', '.css', '/index.tsx', '/index.ts'].map(extension => target + extension).find(candidate => existsSync(candidate) && statSync(candidate).isFile())
    if (found) visit(found)
    else errors.push(`Unresolved relative dependency: ${relative(root, file)} → ${dependency}`)
  }
}
for (const entry of audit.entries) visit(resolve(root, entry))
const sources = readdirSync(base, { recursive: true }).map(file => resolve(base, file)).filter(file => /\.(tsx?|css)$/.test(file) && !/\.(test\.tsx?|d\.ts)$/.test(file) && !file.endsWith('/test-setup.ts'))
const unreachable = sources.filter(file => !reachable.has(file)).map(file => relative(root, file))
const tokens = new Set([...reachable].filter(file => /\.tsx?$/.test(file)).flatMap(file => readFileSync(file, 'utf8').match(/[\w-]+/g) ?? []))
for (const name of audit.retired_classes) if (tokens.has(name)) errors.push(`Retired selector has a possible caller again: ${name}`)
const tokenOwners = sources.filter(file => file.endsWith('.css') && /@theme\b/.test(readFileSync(file, 'utf8'))).map(file => relative(root, file))
if (JSON.stringify(tokenOwners.sort()) !== JSON.stringify([...audit.token_owners].sort())) errors.push('Review competing Tailwind theme ownership')
console.log(JSON.stringify({ kind: 'source reachability only; not rendered/API verification', reachable: reachable.size, unreachable, tokenOwners, errors }, null, 2))
if (unreachable.length || errors.length) process.exitCode = 1
