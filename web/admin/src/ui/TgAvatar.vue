<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{ text?: string; emoji?: string; color?: 1 | 2 | 3 | 4 | 5 | 6 | 7; size?: 28 | 40 | 48 }>(), { size: 40 })

// deterministic color from text (Telegram picks by id)
const c = computed(() => {
  if (props.color) return props.color
  let h = 0
  for (const ch of props.text ?? '') h = (h * 31 + ch.codePointAt(0)!) >>> 0
  return (h % 7) + 1
})
const letter = computed(() => (props.text ?? '').replace(/^@/, '').slice(0, 1))
</script>

<template>
  <div class="tg-av" :class="`tg-av-c${c}`" :style="{ '--s': `${size}px` }" aria-hidden="true">{{ emoji ?? letter }}</div>
</template>
