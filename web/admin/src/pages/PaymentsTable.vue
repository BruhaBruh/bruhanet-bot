<script setup lang="ts">
import { formatDate, formatRub, statusLabels } from '../format'
import type { Payment } from '../types'

defineProps<{ items: Payment[]; showUser?: boolean }>()
</script>

<template>
  <table>
    <thead>
      <tr>
        <th>Создан</th>
        <th v-if="showUser">Юзер</th>
        <th>Месяцев</th>
        <th>Сумма</th>
        <th>Статус</th>
        <th>Решён</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="p in items" :key="p.id">
        <td>{{ formatDate(p.createdAt) }}</td>
        <td v-if="showUser">
          <router-link :to="`/users/${p.telegramId}`">{{ p.username ? '@' + p.username : p.telegramId }}</router-link>
        </td>
        <td>{{ p.month }}</td>
        <td>{{ formatRub(p.price) }}</td>
        <td>{{ statusLabels[p.status] ?? p.status }}</td>
        <td>{{ p.resolvedAt ? formatDate(p.resolvedAt) : '—' }}</td>
      </tr>
      <tr v-if="!items.length"><td colspan="6" class="muted">Платежей нет</td></tr>
    </tbody>
  </table>
</template>
