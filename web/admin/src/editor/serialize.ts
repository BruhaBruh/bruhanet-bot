// Telegram HTML <-> tiptap JSON, превью и санитайзер.
// Строки сообщения = абзацы редактора (склейка через "\n", <br> в Telegram нет);
// <pre> и <blockquote> — блоки, отделённые от соседей одним "\n".
import type { JSONContent } from '@tiptap/core'

type Mark = NonNullable<JSONContent['marks']>[number]

const markTags: Record<string, string> = {
  bold: 'b',
  italic: 'i',
  underline: 'u',
  strike: 's',
  code: 'code',
  spoiler: 'tg-spoiler',
}

const tagMarks: Record<string, string> = {
  b: 'bold', strong: 'bold',
  i: 'italic', em: 'italic',
  u: 'underline', ins: 'underline',
  s: 'strike', strike: 'strike', del: 'strike',
  code: 'code',
  'tg-spoiler': 'spoiler',
}

const escText = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
const escAttr = (s: string) => escText(s).replace(/"/g, '&quot;')
const sameMark = (a: Mark, b: Mark) => JSON.stringify(a) === JSON.stringify(b)

function openTag(m: Mark): string {
  if (m.type === 'link') return `<a href="${escAttr(String(m.attrs?.href ?? ''))}">`
  const t = markTags[m.type]
  return t ? `<${t}>` : ''
}
function closeTag(m: Mark): string {
  const t = m.type === 'link' ? 'a' : markTags[m.type]
  return t ? `</${t}>` : ''
}

function inlineHtml(nodes: JSONContent[]): string {
  let out = ''
  const open: Mark[] = []
  for (const n of nodes) {
    if (n.type !== 'text') continue
    const marks = n.marks ?? []
    let i = 0
    while (i < open.length && i < marks.length && sameMark(open[i], marks[i])) i++
    while (open.length > i) out += closeTag(open.pop()!)
    for (const m of marks.slice(i)) {
      out += openTag(m)
      open.push(m)
    }
    out += escText(n.text ?? '')
  }
  while (open.length) out += closeTag(open.pop()!)
  return out
}

function blockHtml(n: JSONContent): string {
  if (n.type === 'codeBlock') return `<pre>${escText((n.content ?? []).map((c) => c.text ?? '').join(''))}</pre>`
  if (n.type === 'blockquote') return `<blockquote>${(n.content ?? []).map(blockHtml).join('\n')}</blockquote>`
  return inlineHtml(n.content ?? [])
}

export function toTelegramHtml(doc: JSONContent): string {
  return (doc.content ?? []).map(blockHtml).join('\n')
}

const para = (content: JSONContent[]): JSONContent => (content.length ? { type: 'paragraph', content } : { type: 'paragraph' })

function markOf(el: Element): Mark | undefined {
  const tag = el.tagName.toLowerCase()
  if (tag === 'a') {
    const href = el.getAttribute('href')
    return href ? { type: 'link', attrs: { href } } : undefined
  }
  if (tag === 'span') return el.classList.contains('tg-spoiler') ? { type: 'spoiler' } : undefined
  return tagMarks[tag] ? { type: tagMarks[tag] } : undefined
}

function parseBlocks(parent: Node): JSONContent[] {
  const blocks: JSONContent[] = []
  // текущий абзац; null — сразу после блока (следующий "\n" — лишь разделитель)
  let cur: JSONContent[] | null = []
  const flush = () => {
    blocks.push(para(cur ?? []))
    cur = []
  }
  const pushText = (text: string, marks: Mark[]) => {
    cur ??= []
    const last = cur[cur.length - 1]
    if (last && JSON.stringify(last.marks ?? []) === JSON.stringify(marks)) last.text += text
    else cur.push(marks.length ? { type: 'text', text, marks: [...marks] } : { type: 'text', text })
  }
  const walk = (node: Node, marks: Mark[]) => {
    if (node.nodeType === Node.TEXT_NODE) {
      node.textContent!.split('\n').forEach((s, i) => {
        if (i > 0) {
          if (cur === null) cur = []
          else flush()
        }
        if (s) pushText(s, marks)
      })
      return
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return
    const el = node as Element
    const tag = el.tagName.toLowerCase()
    if (!marks.length && (tag === 'pre' || tag === 'blockquote')) {
      if (cur?.length) flush()
      if (tag === 'pre') {
        const text = el.textContent ?? ''
        blocks.push(text ? { type: 'codeBlock', content: [{ type: 'text', text }] } : { type: 'codeBlock' })
      } else {
        const inner = parseBlocks(el)
        blocks.push({ type: 'blockquote', content: inner.length ? inner : [para([])] })
      }
      cur = null
      return
    }
    const m = markOf(el)
    const next = m && !marks.some((x) => x.type === m.type) ? [...marks, m] : marks
    el.childNodes.forEach((c) => walk(c, next))
  }
  parent.childNodes.forEach((c) => walk(c, []))
  if (cur !== null) flush()
  return blocks
}

function parseBody(html: string): HTMLElement {
  return new DOMParser().parseFromString(`<body>${html}</body>`, 'text/html').body
}

export function fromTelegramHtml(html: string): JSONContent {
  return { type: 'doc', content: parseBlocks(parseBody(html)) }
}

// Как html.EscapeString в Go.
const escGo = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&#34;')

const placeholder = /\{\{\s*\.([^{}\s]+)\s*\}\}/g

// Как texts.RenderRaw: значения экранируются, raw — вставляются как HTML, неизвестное — путь.
export function renderPreview(tpl: string, samples: Record<string, string>, raw: string[] = []): string {
  return tpl.replace(placeholder, (_, path: string) => {
    if (raw.includes(path) && path in samples) return samples[path]
    return escGo(path in samples ? samples[path] : path)
  })
}

const simpleTags = new Set(['b', 'strong', 'i', 'em', 'u', 'ins', 's', 'strike', 'del', 'code', 'pre', 'blockquote'])
const dropTags = new Set(['script', 'style', 'template', 'iframe', 'object', 'embed', 'noscript', 'textarea', 'title'])
const schemes = new Set(['http:', 'https:', 'tg:', 'mailto:'])

function safeHref(href: string | null): string | undefined {
  if (!href) return undefined
  try {
    return schemes.has(new URL(href).protocol) ? href : undefined
  } catch {
    return undefined
  }
}

function clean(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return escText(node.textContent ?? '')
  if (node.nodeType !== Node.ELEMENT_NODE) return ''
  const el = node as Element
  const tag = el.tagName.toLowerCase()
  if (dropTags.has(tag)) return ''
  const inner = Array.from(el.childNodes, clean).join('')
  if (tag === 'tg-spoiler' || (tag === 'span' && el.classList.contains('tg-spoiler'))) return `<tg-spoiler>${inner}</tg-spoiler>`
  if (tag === 'a') {
    const href = safeHref(el.getAttribute('href'))
    return href ? `<a href="${escAttr(href)}">${inner}</a>` : `<a>${inner}</a>`
  }
  return simpleTags.has(tag) ? `<${tag}>${inner}</${tag}>` : inner
}

// Оставляет только теги Telegram HTML и href у ссылок (http/https/tg/mailto).
export function sanitize(html: string): string {
  return Array.from(parseBody(html).childNodes, clean).join('')
}

// Как texts.RenderPlain: для кнопок и простых текстов, без экранирования.
export function renderPlain(tpl: string, samples: Record<string, string>): string {
  return tpl.replace(placeholder, (_, path: string) => (path in samples ? samples[path] : path))
}

// Видимый текст сообщения: теги убраны, сущности раскрыты.
export const visibleText = (html: string) => parseBody(html).textContent ?? ''

// Как texts.VisibleLen: длина видимого текста в UTF-16 (JS-строки — уже UTF-16).
export const visibleLen = (html: string) => visibleText(html).length

// Начало текста для списков (рассылки, сообщения юзеру).
export function snippet(html: string, photo?: string): string {
  const t = visibleText(html).trim()
  return (photo ? '🖼 ' : '') + (t ? (t.length > 60 ? t.slice(0, 60) + '…' : t) : 'без текста')
}
