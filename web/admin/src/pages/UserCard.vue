<script setup lang="ts">
import { defineAsyncComponent, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { get } from '../api'
import { snippet } from '../editor/serialize'
import { formatDate, formatRub } from '../format'
import type { Broadcast, DirectMessage, UserDetail } from '../types'
import { TgAvatar, TgButton, TgCell, TgPlaceholder, TgSection } from '../ui'
import PaymentsTable from './PaymentsTable.vue'

// tiptap — отдельным чанком
const Composer = defineAsyncComponent(() => import('../compose/Composer.vue'))

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

const writing = ref(false)
const sent = ref(false)
const statusMark = { pending: '…', sent: '✓', failed: '✗' } as const

function onSent(m: Broadcast | DirectMessage) {
  d.value?.messages.unshift(m as DirectMessage)
  writing.value = false
  sent.value = true
}
</script>

<template>
  <router-link to="/users" class="back">← Юзеры</router-link>
  <p v-if="error" class="err" role="alert">{{ error }}</p>
  <template v-if="d">
    <div class="head">
      <TgAvatar :text="d.user.username || String(d.user.telegramId)" :size="48" />
      <div class="who">
        <h1>{{ d.user.username ? '@' + d.user.username : d.user.telegramId }}</h1>
        <div class="id">{{ d.user.telegramId }}</div>
      </div>
    </div>
    <TgSection header="Подписка">
      <TgCell
        title="Подписка до"
        :after="d.panel.error ? 'панель недоступна' : d.panel.expireAt ? formatDate(d.panel.expireAt) : '—'"
        :after-sub="!d.panel.error && d.panel.status ? d.panel.status : undefined"
      />
    </TgSection>
    <TgSection header="Статистика">
      <TgCell title="С нами с" :after="formatDate(d.user.createdAt)" />
      <TgCell title="Подтверждённых оплат" :after="`${d.stats.confirmed} на ${formatRub(d.stats.confirmedSum)}`" />
      <TgCell title="Неподтверждённых авансов" :after="String(d.stats.unpaidAdvances)" />
      <TgCell title="Первая оплата" :after="d.stats.firstConfirmedAt ? formatDate(d.stats.firstConfirmedAt) : '—'" />
      <TgCell title="Последняя оплата" :after="d.stats.lastConfirmedAt ? formatDate(d.stats.lastConfirmedAt) : '—'" />
    </TgSection>
    <PaymentsTable header="Платежи" :items="d.payments" />
    <TgSection header="Сообщения">
      <div class="pad">
        <Composer v-if="writing" mode="direct" :user-id="d.user.telegramId" @sent="onSent" />
        <template v-else>
          <TgButton mode="bezeled" @click="(writing = true), (sent = false)">Написать</TgButton>
          <span v-if="sent" class="ok" role="status">Отправлено</span>
        </template>
      </div>
      <TgPlaceholder v-if="!d.messages.length" emoji="✉️" title="Сообщений нет" />
      <TgCell
        v-for="m in d.messages"
        :key="m.id"
        :title="snippet(m.text, m.photo)"
        :subtitle="formatDate(m.createdAt)"
        :after="statusMark[m.status] ?? m.status"
      />
    </TgSection>
  </template>
</template>

<style scoped>
.back { display: inline-block; margin: 0 0 12px 16px; }
.head { display: flex; align-items: center; gap: 16px; margin-bottom: 24px; }
h1 { margin: 0; font-size: 20px; }
.id { color: var(--tg-hint-text); font-size: 13px; }
.err { color: var(--tg-danger-text); }
.pad { display: flex; align-items: center; gap: 12px; padding: 12px 16px; }
.pad > :deep(.composer) { flex: 1; min-width: 0; }
.ok { color: var(--tg-link); }
</style>
