// @vitest-environment jsdom
import type { JSONContent } from '@tiptap/core'
import { describe, expect, it } from 'vitest'
import { fromTelegramHtml, renderPlain, renderPreview, sanitize, toTelegramHtml } from './serialize'

const p = (...content: JSONContent[]): JSONContent => (content.length ? { type: 'paragraph', content } : { type: 'paragraph' })
const t = (text: string, ...marks: JSONContent['marks'] & {}): JSONContent => (marks.length ? { type: 'text', text, marks } : { type: 'text', text })
const doc = (...content: JSONContent[]): JSONContent => ({ type: 'doc', content })

describe('toTelegramHtml / fromTelegramHtml', () => {
  const cases: [string, JSONContent, string][] = [
    ['bold', doc(p(t('Здравствуйте, '), t('{{.name}}', { type: 'bold' }))), 'Здравствуйте, <b>{{.name}}</b>'],
    ['italic', doc(p(t('a', { type: 'italic' }), t(' b'))), '<i>a</i> b'],
    ['nested', doc(p(t('a', { type: 'bold' }), t('b', { type: 'bold' }, { type: 'italic' }))), '<b>a<i>b</i></b>'],
    ['underline+strike', doc(p(t('u', { type: 'underline' }), t('s', { type: 'strike' }))), '<u>u</u><s>s</s>'],
    ['link', doc(p(t('сайт', { type: 'link', attrs: { href: 'https://x.ru/?a=1&b="2"' } }))), '<a href="https://x.ru/?a=1&amp;b=&quot;2&quot;">сайт</a>'],
    ['code', doc(p(t('на номер '), t('{{.phone}}', { type: 'code' }))), 'на номер <code>{{.phone}}</code>'],
    ['spoiler', doc(p(t('секрет', { type: 'spoiler' }))), '<tg-spoiler>секрет</tg-spoiler>'],
    ['escaping', doc(p(t('1 < 2 & 3 > 0'))), '1 &lt; 2 &amp; 3 &gt; 0'],
    ['lines', doc(p(t('Заголовок', { type: 'bold' })), p(), p(t('текст'))), '<b>Заголовок</b>\n\nтекст'],
    ['pre', doc(p(t('a')), { type: 'codeBlock', content: [t('x\ny')] }, p(t('b'))), 'a\n<pre>x\ny</pre>\nb'],
    ['blockquote', doc({ type: 'blockquote', content: [p(t('q1')), p(t('q2'))] }), '<blockquote>q1\nq2</blockquote>'],
    ['empty', doc(p()), ''],
  ]
  for (const [name, d, html] of cases) {
    it(name, () => {
      expect(toTelegramHtml(d)).toBe(html)
      expect(fromTelegramHtml(html)).toEqual(d)
    })
  }

  it('parses tag aliases', () => {
    expect(fromTelegramHtml('<strong>a</strong><em>b</em><ins>c</ins><del>d</del><span class="tg-spoiler">e</span>')).toEqual(
      doc(p(t('a', { type: 'bold' }), t('b', { type: 'italic' }), t('c', { type: 'underline' }), t('d', { type: 'strike' }), t('e', { type: 'spoiler' }))),
    )
  })

  it('round-trips the default requisites text', () => {
    const html = '<b>Оплата тарифа на {{.months}} за {{.price}} ₽</b>\n\n<b>Для оплаты</b>\nПереведите <b>{{.price}} ₽</b>\nна номер <code>{{.phone}}</code> в <b>{{.bank}}</b>\nс одним из комментариев:\n{{.comments}}\nи отправьте в чат фото платежа'
    expect(toTelegramHtml(fromTelegramHtml(html))).toBe(html)
  })
})

describe('renderPreview', () => {
  it('escapes values, keeps raw values, renders unknown as path', () => {
    const out = renderPreview('Hi {{.name}} {{ .list }} {{.foo.bar}}', { name: '<Иван & co>', list: '<code>a</code>' }, ['list'])
    expect(out).toBe('Hi &lt;Иван &amp; co&gt; <code>a</code> foo.bar')
  })
  it('escapes non-raw vars even when they look like html', () => {
    expect(renderPreview('{{.x}}', { x: '<b>' })).toBe('&lt;b&gt;')
  })
})

describe('renderPlain', () => {
  it('substitutes without escaping, unknown as path', () => {
    expect(renderPlain('{{.months}} за {{.price}} ₽ {{.a.b}}', { months: '3 месяца', price: '<500>' })).toBe('3 месяца за <500> ₽ a.b')
  })
})

describe('sanitize', () => {
  it('drops script and disallowed attributes', () => {
    expect(sanitize('<b onclick="x()">a</b><script>alert(1)</script><a href="https://x.ru" style="c" target="_blank">l</a>')).toBe(
      '<b>a</b><a href="https://x.ru">l</a>',
    )
  })
  it('unwraps unknown tags and bad links', () => {
    expect(sanitize('<div><img src=x onerror=alert(1)>t</div><a href="javascript:alert(1)">x</a>')).toBe('t<a>x</a>')
  })
  it('keeps spoilers', () => {
    expect(sanitize('<span class="tg-spoiler">s</span><tg-spoiler>t</tg-spoiler>')).toBe('<tg-spoiler>s</tg-spoiler><tg-spoiler>t</tg-spoiler>')
  })
})
