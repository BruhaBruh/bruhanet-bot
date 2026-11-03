<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { get } from '../api'
import Composer from '../compose/Composer.vue'
import { snippet } from '../editor/serialize'
import { formatDate } from '../format'
import type { Broadcast } from '../types'
import { TgCell, TgPlaceholder, TgSection, TgSpinner } from '../ui'

const items = ref<Broadcast[]>()
const loadError = ref(false)

// ponytail: только первая страница (20 последних), пагинация — когда история понадобится глубже
async function load() {
  try {
    items.value = (await get<{ items: Broadcast[] }>('/api/broadcasts?page=1')).items
    loadError.value = false
  } catch {
    loadError.value = true
  }
}

// автообновление, пока есть незавершённые и вкладка видна
const timer = setInterval(() => {
  if (document.visibilityState === 'visible' && items.value?.some((b) => !b.finishedAt)) load()
}, 5000)
onMounted(load)
onBeforeUnmount(() => clearInterval(timer))

const stats = (b: Broadcast) => `доставлено ${b.sent} из ${b.total}, не доставлено ${b.failed}`
</script>

<template>
  <h1 class="tg-visually-hidden">Рассылки</h1>
  <TgSection header="Новая рассылка">
    <div class="pad"><Composer mode="broadcast" @sent="load" /></div>
  </TgSection>

  <TgSection header="История">
    <p v-if="loadError" class="tg-err" role="alert">Не удалось загрузить</p>
    <div v-else-if="!items" class="tg-loading" role="status"><TgSpinner /><span class="tg-visually-hidden">Загрузка…</span></div>
    <TgPlaceholder v-else-if="!items.length" emoji="📣" title="Рассылок ещё не было" />
    <TgCell
      v-for="b in items"
      v-else
      :key="b.id"
      :title="snippet(b.text, b.photo)"
      :subtitle="formatDate(b.createdAt) + ' · ' + (b.finishedAt ? stats(b) : 'идёт… ' + stats(b))"
    />
  </TgSection>
</template>

<style scoped>
.pad { padding: 12px 16px; }
</style>
