<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { get } from '../api'
import { formatRub, statusLabels } from '../format'
import type { Payment } from '../types'
import PaymentsTable from './PaymentsTable.vue'

const pageSize = 50
const status = ref('')
const from = ref('')
const to = ref('')
const page = ref(1)
const items = ref<Payment[]>([])
const total = ref(0)
const revenue = ref(0)
const error = ref(false)
const pages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

async function load() {
  const p = new URLSearchParams({ status: status.value, page: String(page.value) })
  // пустые from/to → сервер берёт последние 30 дней МСК
  if (from.value) p.set('from', from.value)
  if (to.value) p.set('to', to.value)
  try {
    const r = await get<{ items: Payment[]; total: number; revenue: number }>(`/api/payments?${p}`)
    items.value = r.items
    total.value = r.total
    revenue.value = r.revenue
    error.value = false
  } catch {
    error.value = true
  }
}

function apply() {
  page.value = 1
  load()
}
function go(n: number) {
  page.value = n
  load()
}
onMounted(load)
</script>

<template>
  <h2>Платежи</h2>
  <div class="filters">
    <select v-model="status" @change="apply">
      <option value="">все статусы</option>
      <option v-for="(label, k) in statusLabels" :key="k" :value="k">{{ label }}</option>
    </select>
    <label>с <input v-model="from" type="date" @change="apply" /></label>
    <label>по <input v-model="to" type="date" @change="apply" /></label>
  </div>
  <p v-if="error" class="err">Не удалось загрузить</p>
  <p>Выручка (подтверждено за период): <b>{{ formatRub(revenue) }}</b> · всего платежей: {{ total }}</p>
  <PaymentsTable :items="items" show-user />
  <div class="pager">
    <button :disabled="page <= 1" @click="go(page - 1)">←</button>
    <span>{{ page }} / {{ pages }}</span>
    <button :disabled="page >= pages" @click="go(page + 1)">→</button>
  </div>
</template>

<style scoped>
.filters { display: flex; gap: 12px; margin: 8px 0 16px; align-items: center; }
.pager { display: flex; gap: 12px; align-items: center; margin-top: 12px; }
</style>
