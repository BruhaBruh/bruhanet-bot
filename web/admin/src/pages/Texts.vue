<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ApiError, del, get, post, put } from '../api'
import TgEditor from '../editor/TgEditor.vue'
import { renderPlain, renderPreview, sanitize } from '../editor/serialize'
import type { TextItem } from '../types'
import { TgButton, TgCell, TgInput, TgPlaceholder, TgSection, TgSpinner } from '../ui'

const route = useRoute()
const items = ref<TextItem[]>([])
const loaded = ref(false)
const loadError = ref(false)
const q = ref('')
// черновики по ключу: переключение между текстами не теряет правки
const drafts = reactive<Record<string, string>>({})
const error = ref('')
const sent = ref(false)
const busy = ref(false)
const editor = ref<InstanceType<typeof TgEditor>>()
const plainBox = ref<HTMLElement>()
let sentTimer: ReturnType<typeof setTimeout> | undefined

onMounted(async () => {
  try {
    items.value = (await get<{ items: TextItem[] }>('/api/texts')).items
    loaded.value = true
  } catch {
    loadError.value = true
  }
})

const cur = computed(() => items.value.find((i) => i.key === route.query.key) ?? items.value[0])
const draft = computed({
  get: () => (cur.value ? (drafts[cur.value.key] ?? cur.value.body) : ''),
  set: (v: string) => {
    if (cur.value) drafts[cur.value.key] = v
  },
})
const isDirty = (i: TextItem) => drafts[i.key] !== undefined && drafts[i.key] !== i.body

watch(() => cur.value?.key, () => {
  error.value = ''
  sent.value = false
})

const groups = computed(() => {
  const s = q.value.trim().toLowerCase()
  const out = new Map<string, TextItem[]>()
  for (const i of items.value) {
    if (s && !i.title.toLowerCase().includes(s) && !i.body.toLowerCase().includes(s)) continue
    out.set(i.group, [...(out.get(i.group) ?? []), i])
  }
  return [...out]
})

const samples = computed(() => Object.fromEntries((cur.value?.vars ?? []).map((v) => [v.name, v.sample])))
const preview = computed(() => {
  const c = cur.value
  if (!c) return ''
  if (c.kind !== 'message') return renderPlain(draft.value, samples.value)
  const raw = c.vars.filter((v) => v.raw).map((v) => v.name)
  return sanitize(renderPreview(draft.value, samples.value, raw))
})

const varLabel = (name: string) => `{{.${name}}}`

function insertVar(name: string) {
  const text = varLabel(name)
  if (cur.value?.kind === 'message') return editor.value?.insert(text)
  const input = plainBox.value?.querySelector('input')
  const at = input?.selectionStart ?? draft.value.length
  const end = input?.selectionEnd ?? at
  draft.value = draft.value.slice(0, at) + text + draft.value.slice(end)
}

async function run(fn: () => Promise<void>) {
  error.value = ''
  sent.value = false
  busy.value = true
  try {
    await fn()
  } catch (e) {
    error.value = e instanceof ApiError && e.body.message ? e.body.message : 'Не удалось выполнить, попробуйте ещё раз'
  } finally {
    busy.value = false
  }
}

const path = (c: TextItem) => `/api/texts/${encodeURIComponent(c.key)}`

const save = (c: TextItem) =>
  run(async () => {
    const body = draft.value
    await put(path(c), { body })
    c.body = body
    c.overridden = body !== c.default
    delete drafts[c.key]
  })

const sendTest = (c: TextItem) =>
  run(async () => {
    await post(`${path(c)}/test`, { body: draft.value })
    sent.value = true
    clearTimeout(sentTimer)
    sentTimer = setTimeout(() => (sent.value = false), 3000)
  })

const reset = (c: TextItem) =>
  run(async () => {
    await del(path(c))
    c.body = c.default
    c.overridden = false
    delete drafts[c.key]
  })

function cancel(c: TextItem) {
  delete drafts[c.key]
  error.value = ''
}
</script>

<template>
  <h1 class="tg-visually-hidden">Тексты</h1>
  <p v-if="loadError" class="tg-err" role="alert">Не удалось загрузить</p>
  <div v-else-if="!loaded" class="tg-loading" role="status"><TgSpinner /><span class="tg-visually-hidden">Загрузка…</span></div>
  <div v-else class="texts">
    <div class="list">
      <TgInput v-model="q" icon="search" placeholder="Поиск" class="search" />
      <TgPlaceholder v-if="!groups.length" emoji="🔍" title="Ничего не найдено" />
      <TgSection v-for="[group, list] in groups" :key="group" :header="group">
        <TgCell
          v-for="i in list"
          :key="i.key"
          :to="{ query: { key: i.key } }"
          :title="i.title"
          :subtitle="isDirty(i) ? 'есть несохранённые изменения' : i.overridden ? 'изменён' : undefined"
          :active="i.key === cur?.key"
        />
      </TgSection>
    </div>

    <div v-if="cur" class="edit">
      <h2>{{ cur.title }}</h2>
      <p class="meta">
        <span v-if="cur.overridden">изменён</span>
        <span v-if="isDirty(cur)" class="dirty">есть несохранённые изменения</span>
      </p>

      <TgEditor v-if="cur.kind === 'message'" :key="cur.key" ref="editor" v-model="draft" />
      <div v-else ref="plainBox">
        <TgInput v-model="draft" :placeholder="cur.title" maxlength="64" />
        <p class="hint">Простой текст без разметки, до 64 символов.</p>
      </div>

      <template v-if="cur.vars.length">
        <div class="label">Переменные (клик — вставить)</div>
        <div class="vars">
          <TgButton v-for="v in cur.vars" :key="v.name" mode="bezeled" size="s" :title="v.description" @click="insertVar(v.name)">
            {{ varLabel(v.name) }}
          </TgButton>
        </div>
      </template>

      <div class="label">Превью</div>
      <!-- превью сообщения: только разрешённые теги после sanitize -->
      <div v-if="cur.kind === 'message'" class="tg-msg" v-html="preview" />
      <div v-else class="tg-msg">{{ preview }}</div>

      <p v-if="error" class="tg-err" role="alert">{{ error }}</p>
      <p v-if="sent" class="tg-ok" role="status">Отправлено в Telegram</p>

      <div class="actions">
        <TgButton :loading="busy" :disabled="!isDirty(cur)" @click="save(cur)">Сохранить</TgButton>
        <TgButton mode="bezeled" :disabled="busy" @click="sendTest(cur)">Отправить себе</TgButton>
        <TgButton v-if="isDirty(cur)" mode="plain" :disabled="busy" @click="cancel(cur)">Отменить</TgButton>
        <TgButton v-if="cur.overridden" mode="plain" :disabled="busy" @click="reset(cur)">Сбросить к стандартному</TgButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.texts { display: flex; gap: 16px; align-items: flex-start; }
.list { width: 220px; flex: none; }
.edit { flex: 1; min-width: 0; }
.search { margin-bottom: 16px; }
h2 { margin: 0; font-size: 20px; }
.meta { display: flex; gap: 8px; margin: 4px 0 12px; font-size: 13px; color: var(--tg-hint-text); min-height: 1em; }
.dirty { color: var(--tg-link); }
.label { font-size: 13px; color: var(--tg-hint-text); margin: 16px 0 6px; }
.vars, .actions { display: flex; gap: 6px; flex-wrap: wrap; }
.actions { margin-top: 16px; }
.hint { color: var(--tg-hint-text); font-size: 13px; margin: 6px 16px 0; }
/* узкая колонка: длинные названия переносятся, а не обрезаются */
.list :deep(.tg-title) { white-space: normal; }
@media (max-width: 700px) {
  .texts { flex-direction: column; }
  .list { width: 100%; }
}
</style>
