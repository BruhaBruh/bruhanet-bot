<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { get } from '../api'
import { formatMonthYear, formatRub, plural } from '../format'
import type { UserSummary } from '../types'
import { TgAvatar, TgCell, TgInput, TgPlaceholder, TgSection, TgSpinner } from '../ui'

const q = ref('')
const items = ref<UserSummary[]>([])
const error = ref(false)
const loaded = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0

async function load() {
  const n = ++seq
  try {
    const r = await get<{ items: UserSummary[] }>(`/api/users?q=${encodeURIComponent(q.value.trim())}`)
    if (n === seq) {
      items.value = r.items
      error.value = false
      loaded.value = true
    }
  } catch {
    if (n === seq) error.value = true
  }
}

watch(q, () => {
  clearTimeout(timer)
  timer = setTimeout(load, 300)
})
onMounted(load)
</script>

<template>
  <h1 class="tg-visually-hidden">Юзеры</h1>
  <TgInput v-model="q" icon="search" placeholder="ID или @username" class="search" />
  <p v-if="error" class="err" role="alert">Не удалось загрузить</p>
  <div v-if="!loaded && !error" class="loading" role="status" ><TgSpinner /><span class="tg-visually-hidden">Загрузка…</span></div>
  <TgPlaceholder v-else-if="!items.length && !error" emoji="🔍" title="Никого нет" />
  <TgSection v-else-if="loaded" :header="`Найдено: ${items.length}`">
    <TgCell
      v-for="u in items"
      :key="u.telegramId"
      :to="`/users/${u.telegramId}`"
      :title="u.username ? '@' + u.username : String(u.telegramId)"
      :subtitle="`${u.telegramId} · с ${formatMonthYear(u.createdAt)}`"
      :after="formatRub(u.confirmedSum)"
      :after-sub="`${u.confirmed} ${plural(u.confirmed, ['оплата', 'оплаты', 'оплат'])}`"
      chevron
    >
      <template #before><TgAvatar :text="u.username || String(u.telegramId)" /></template>
    </TgCell>
  </TgSection>
</template>

<style scoped>
.search { margin-bottom: 20px; }
.loading { display: flex; justify-content: center; padding: 32px; color: var(--tg-hint-text); }
.err { color: var(--tg-danger-text); }
</style>
