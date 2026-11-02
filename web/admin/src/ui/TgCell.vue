<script setup lang="ts">
import { RouterLink, type RouteLocationRaw } from 'vue-router'

defineProps<{
  title: string
  subtitle?: string
  after?: string
  afterSub?: string
  to?: RouteLocationRaw
  chevron?: boolean
  active?: boolean
}>()
</script>

<template>
  <component :is="to ? RouterLink : 'div'" :to="to" class="tg-cell" :class="{ 'tg-active': active }">
    <slot name="before" />
    <div class="tg-cell-body">
      <div class="tg-title">{{ title }}</div>
      <div v-if="subtitle" class="tg-sub">{{ subtitle }}</div>
    </div>
    <div v-if="after || afterSub || $slots.after" class="tg-cell-after">
      <slot name="after">
        <b v-if="after" class="tg-ellipsis">{{ after }}</b>
        <span v-if="afterSub" class="tg-sub">{{ afterSub }}</span>
      </slot>
    </div>
    <span v-if="chevron" class="tg-chev" aria-hidden="true">›</span>
  </component>
</template>
