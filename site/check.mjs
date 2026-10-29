// Kontrola webu: každý lokálny odkaz (href, src, url()) vedie na existujúci
// súbor alebo kotvu na stránke. Spustenie: node site/check.mjs
import { readFileSync, existsSync, readdirSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const dir = dirname(fileURLToPath(import.meta.url))
const errors = []
const pages = readdirSync(dir).filter((f) => f.endsWith('.html'))
const ids = Object.fromEntries(pages.map((p) => [p, new Set([...readFileSync(join(dir, p), 'utf8').matchAll(/id="([^"]+)"/g)].map((m) => m[1]))]))

for (const file of [...pages, 'style.css']) {
  const text = readFileSync(join(dir, file), 'utf8')
  const refs = [...text.matchAll(/(?:href|src)="([^"]+)"|url\('([^']+)'\)/g)].map((m) => m[1] ?? m[2])
  for (const ref of refs) {
    if (/^(https?:|mailto:|data:)/.test(ref)) continue
    const [path, hash] = ref.split('#')
    const target = path === '' || path === './' ? (path === '' ? file : 'index.html') : path
    if (!existsSync(join(dir, target))) errors.push(`${file}: chýba ${ref}`)
    else if (hash && target.endsWith('.html') && !ids[target]?.has(hash)) errors.push(`${file}: chýba kotva ${ref}`)
  }
  if (file.endsWith('.html') && !/<html lang="sk">/.test(text)) errors.push(`${file}: chýba lang="sk"`)
}
if (errors.length) {
  console.error(errors.join('\n'))
  process.exit(1)
}
console.log(`web v poriadku (stránky: ${pages.length})`)
