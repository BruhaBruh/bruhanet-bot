<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { get } from '../api'
import { formatDate, formatRub } from '../format'
import type { UserDetail } from '../types'
import PaymentsTable from './PaymentsTable.vue'

const route = useRoute()
const d = ref<UserDetail>()
const error = ref('')

onMounted(async () => {
  try {
    d.value = await get<UserDetail>(`/api/users/${route.params.id}`)
  } catch (e) {
    error.value = String(e).includes('404') ? 'Юзер не найден' : 'Не удалось загрузить'
  }
})
</script>

<template>
  <router-link to="/users">← Юзеры</router-link>
  <p v-if="error" class="err">{{ error }}</p>
  <template v-if="d">
    <h2>{{ d.user.username ? '@' + d.user.username : d.user.telegramId }} <small class="muted">{{ d.user.telegramId }}</small></h2>
    <p>
      Подписка до:
      <b v-if="d.panel.error">панель недоступна</b>
      <b v-else>{{ d.panel.expireAt ? formatDate(d.panel.expireAt) : '—' }}</b>
      <span v-if="d.panel.status" class="muted"> ({{ d.panel.status }})</span>
    </p>
    <ul>
      <li>С нами с: {{ formatDate(d.user.createdAt) }}</li>
      <li>Подтверждённых оплат: {{ d.stats.confirmed }} на {{ formatRub(d.stats.confirmedSum) }}</li>
      <li>Неподтверждённых авансов: {{ d.stats.unpaidAdvances }}</li>
      <li>Первая оплата: {{ d.stats.firstConfirmedAt ? formatDate(d.stats.firstConfirmedAt) : '—' }}</li>
      <li>Последняя оплата: {{ d.stats.lastConfirmedAt ? formatDate(d.stats.lastConfirmedAt) : '—' }}</li>
    </ul>
    <h3>Платежи</h3>
    <PaymentsTable :items="d.payments" />
  </template>
</template>
