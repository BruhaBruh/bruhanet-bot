<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ApiError, get, put } from '../api'
import type { Settings } from '../types'
import { TgButton, TgInput, TgSection, TgSpinner } from '../ui'

const tariffs = ref<{ months: string; price: string }[]>([])
const phone = ref('')
const bank = ref('')
const comments = ref<string[]>([])
const supportUrl = ref('')
const loaded = ref(false)
const loadError = ref(false)
const busy = ref(false)
const saved = ref(false)
const error = ref('')
const problems = ref<string[]>([])
const storedProblems = ref<string[] | null>(null)

onMounted(async () => {
  try {
    const s = await get<Settings>('/api/settings')
    tariffs.value = Object.entries(s.prices)
      .sort(([a], [b]) => Number(a) - Number(b))
      .map(([months, price]) => ({ months, price: String(price) }))
    phone.value = s.phone
    bank.value = s.bank
    comments.value = [...s.comments]
    supportUrl.value = s.supportUrl
    if (s.usingDefaults) storedProblems.value = s.problems ?? []
    loaded.value = true
  } catch {
    loadError.value = true
  }
})

async function save() {
  error.value = ''
  problems.value = []
  saved.value = false
  // одинаковые сроки схлопнулись бы в один ключ объекта — ловим до отправки
  const dup = tariffs.value.map((t) => t.months.trim()).find((m, i, a) => a.indexOf(m) !== i)
  if (dup !== undefined) {
    error.value = `Тариф на ${dup || '(пусто)'} мес. указан дважды`
    return
  }
  const bad = tariffs.value.find((t) => !/^\d+$/.test(t.months.trim()) || +t.months < 1 || +t.months > 120 || !/^\d+$/.test(t.price.trim()) || +t.price < 1)
  if (bad) {
    error.value = 'Срок тарифа — целое число от 1 до 120 месяцев, цена — целое число больше 0'
    return
  }
  const body: Settings = {
    prices: Object.fromEntries(tariffs.value.map((t) => [t.months.trim(), Number(t.price)])),
    phone: phone.value,
    bank: bank.value,
    comments: comments.value,
    supportUrl: supportUrl.value,
  }
  busy.value = true
  try {
    await put('/api/settings', body)
    saved.value = true
    storedProblems.value = null
  } catch (e) {
    if (e instanceof ApiError && e.body.message) {
      error.value = e.body.message
      problems.value = e.body.problems ?? []
    } else error.value = 'Не удалось сохранить, попробуйте ещё раз'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <h1 class="tg-visually-hidden">Настройки</h1>
  <p v-if="loadError" class="err" role="alert">Не удалось загрузить</p>
  <div v-else-if="!loaded" class="loading" role="status"><TgSpinner /><span class="tg-visually-hidden">Загрузка…</span></div>
  <form v-else @submit.prevent="save" @input="saved = false">
    <p v-if="storedProblems" class="warn" role="alert">
      Сохранённые настройки некорректны, бот работает на значениях из .env: {{ storedProblems.join('; ') }}. Сохраните форму, чтобы применить новые.
    </p>
    <TgSection header="Тарифы" footer="Месяцев и цена в рублях. Хотя бы один тариф.">
      <div v-for="(t, i) in tariffs" :key="i" class="row">
        <TgInput v-model="t.months" grouped type="number" min="1" placeholder="Месяцев" />
        <TgInput v-model="t.price" grouped type="number" min="1" placeholder="Цена, ₽" />
        <TgButton mode="plain" :disabled="tariffs.length <= 1" aria-label="Удалить тариф" @click="tariffs.splice(i, 1)">✕</TgButton>
      </div>
    </TgSection>
    <TgButton class="add" mode="bezeled" size="s" @click="tariffs.push({ months: '', price: '' })">Добавить тариф</TgButton>

    <TgSection header="Реквизиты">
      <div class="col">
        <TgInput v-model="phone" grouped placeholder="Телефон" />
        <TgInput v-model="bank" grouped placeholder="Банк" />
      </div>
    </TgSection>

    <TgSection header="Комментарии к переводу">
      <div v-for="(_, i) in comments" :key="i" class="row">
        <TgInput v-model="comments[i]" grouped placeholder="Комментарий" />
        <TgButton mode="plain" aria-label="Удалить комментарий" @click="comments.splice(i, 1)">✕</TgButton>
      </div>
    </TgSection>
    <TgButton class="add" mode="bezeled" size="s" @click="comments.push('')">Добавить комментарий</TgButton>

    <TgSection header="Поддержка">
      <TgInput v-model="supportUrl" grouped type="url" placeholder="https://t.me/…" aria-label="Ссылка на поддержку" />
    </TgSection>

    <div v-if="error" class="err" role="alert">
      <p v-if="problems.length <= 1">{{ error }}</p>
      <ul v-else>
        <li v-for="p in problems" :key="p">{{ p }}</li>
      </ul>
    </div>
    <p v-if="saved" class="ok" role="status">Сохранено</p>
    <TgButton :loading="busy" @click="save">Сохранить</TgButton>
  </form>
</template>

<style scoped>
.row { display: flex; align-items: center; gap: 1px; background: var(--tg-sep); }
.row > :deep(.tg-field) { flex: 1; min-width: 0; }
.row > .tg-btn { background: var(--tg-bg); border-radius: 0; align-self: stretch; }
.row + .row { margin-top: 1px; }
.col { display: flex; flex-direction: column; gap: 1px; background: var(--tg-sep); }
.add { margin: -16px 0 24px 16px; }
.loading { display: flex; justify-content: center; padding: 32px; color: var(--tg-hint-text); }
.err { color: var(--tg-danger-text); }
.err p { margin: 0 0 8px; }
.warn { color: var(--tg-danger-text); margin: 0 16px 16px; }
.ok { color: var(--tg-link); }
</style>
