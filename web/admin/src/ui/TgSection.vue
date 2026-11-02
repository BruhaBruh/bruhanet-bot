<script setup lang="ts">
defineProps<{ header?: string; footer?: string }>()
</script>

<template>
  <section class="tg-section">
    <div v-if="header" class="tg-section-h">{{ header }}</div>
    <div class="tg-section-body"><slot /></div>
    <div v-if="footer" class="tg-section-f">{{ footer }}</div>
  </section>
</template>
