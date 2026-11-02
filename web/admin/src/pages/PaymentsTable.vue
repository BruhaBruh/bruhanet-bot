<script setup lang="ts">
import { formatDate, formatRub, statusLabels } from '../format'
import type { Payment } from '../types'
import { TgCell, TgPlaceholder, TgSection } from '../ui'

defineProps<{ items: Payment[]; showUser?: boolean; header?: string }>()
</script>

<template>
  <TgSection :header="header">
    <TgPlaceholder v-if="!items.length" emoji="💳" title="Платежей нет" />
    <TgCell
      v-for="p in items"
      :key="p.id"
      :title="(showUser ? (p.username ? '@' + p.username : String(p.telegramId)) + ' · ' : '') + p.month + ' мес'"
      :subtitle="formatDate(p.createdAt) + ' · ' + (statusLabels[p.status] ?? p.status)"
      :after="formatRub(p.price)"
      :to="showUser ? `/users/${p.telegramId}` : undefined"
      :chevron="showUser"
    />
  </TgSection>
</template>
