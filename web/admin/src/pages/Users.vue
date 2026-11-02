<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { get } from '../api'
import { formatDate, formatRub } from '../format'
import type { UserSummary } from '../types'

const q = ref('')
const items = ref<UserSummary[]>([])
const error = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0

async function load() {
  const n = ++seq
  try {
    const r = await get<{ items: UserSummary[] }>(`/api/users?q=${encodeURIComponent(q.value.trim())}`)
    if (n === seq) {
      items.value = r.items
      error.value = false
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
  <h2>Юзеры</h2>
  <input v-model="q" placeholder="ID или @username" />
  <p v-if="error" class="err">Не удалось загрузить</p>
  <table>
    <thead>
      <tr><th>ID</th><th>@username</th><th>Оплат</th><th>Сумма</th><th>С нами с</th></tr>
    </thead>
    <tbody>
      <tr v-for="u in items" :key="u.telegramId">
        <td><router-link :to="`/users/${u.telegramId}`">{{ u.telegramId }}</router-link></td>
        <td>{{ u.username ? '@' + u.username : '—' }}</td>
        <td>{{ u.confirmed }}</td>
        <td>{{ formatRub(u.confirmedSum) }}</td>
        <td>{{ formatDate(u.createdAt) }}</td>
      </tr>
      <tr v-if="!items.length"><td colspan="5" class="muted">Никого нет</td></tr>
    </tbody>
  </table>
</template>

<style scoped>
input { margin: 8px 0 16px; width: 280px; }
</style>
