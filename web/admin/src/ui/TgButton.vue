<script setup lang="ts">
import TgSpinner from './TgSpinner.vue'

withDefaults(defineProps<{ mode?: 'filled' | 'plain' | 'bezeled'; size?: 's' | 'm' | 'l'; loading?: boolean; disabled?: boolean }>(), {
  mode: 'filled',
  size: 'm',
})
</script>

<template>
  <button
    type="button"
    class="tg-btn"
    :class="[`tg-btn-${mode}`, `tg-btn-${size}`, { 'tg-btn-loading': loading }]"
    :disabled="disabled || loading"
    :aria-busy="loading ? 'true' : undefined"
  >
    <span class="tg-btn-content"><slot /></span>
    <TgSpinner v-if="loading" />
  </button>
</template>
