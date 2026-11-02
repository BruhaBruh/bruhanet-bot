<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { get } from '../api'
import { formatRub, mskDate, statusLabels } from '../format'
import type { Payment } from '../types'
import { TgButton, TgCell, TgInput, TgSection, TgSelect } from '../ui'
import PaymentsTable from './PaymentsTable.vue'

const pageSize = 50
const status = ref('')
// период всегда виден и уходит явно: сегодня и 29 дней назад (МСК), как default сервера
const from = ref(mskDate(new Date(), -29))
const to = ref(mskDate(new Date()))
const page = ref(1)
const items = ref<Payment[]>([])
const total = ref(0)
const revenue = ref(0)
const error = ref(false)
const statusOptions = [{ value: '', label: 'все статусы' }, ...Object.entries(statusLabels).map(([value, label]) => ({ value, label }))]
const pages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

async function load() {
  const p = new URLSearchParams({ status: status.value, page: String(page.value) })
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
  <h1 class="tg-visually-hidden">Платежи</h1>
  <TgSection header="Фильтры">
    <div class="filters">
      <TgSelect grouped v-model="status" :options="statusOptions" aria-label="Статус" @change="apply" />
      <TgInput grouped v-model="from" type="date" aria-label="С даты" @change="apply" />
      <TgInput grouped v-model="to" type="date" aria-label="По дату" @change="apply" />
    </div>
  </TgSection>
  <p v-if="error" class="err" role="alert">Не удалось загрузить</p>
  <TgSection header="Выручка (по дате подтверждения за период)">
    <TgCell title="Выручка" :after="formatRub(revenue)" :after-sub="`всего платежей: ${total}`" />
  </TgSection>
  <PaymentsTable header="Платежи" :items="items" show-user />
  <p class="hint">Список отфильтрован по дате создания платежа.</p>
  <div class="pager">
    <TgButton mode="bezeled" :disabled="page <= 1" aria-label="Назад" @click="go(page - 1)">←</TgButton>
    <span>{{ page }} / {{ pages }}</span>
    <TgButton mode="bezeled" :disabled="page >= pages" aria-label="Вперёд" @click="go(page + 1)">→</TgButton>
  </div>
</template>

<style scoped>
.filters { display: flex; flex-direction: column; gap: 1px; background: var(--tg-sep); }
.hint { color: var(--tg-hint-text); font-size: 13px; margin: 0 16px; }
.pager { display: flex; gap: 12px; align-items: center; justify-content: center; margin-top: 12px; }
.err { color: var(--tg-danger-text); }
</style>
