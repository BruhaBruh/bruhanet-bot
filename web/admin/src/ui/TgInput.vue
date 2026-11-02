<script setup lang="ts">
import { computed, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })
defineProps<{ modelValue: string; placeholder?: string; type?: string; icon?: 'search'; grouped?: boolean }>()
defineEmits<{ 'update:modelValue': [string]; change: [] }>()

// class/style stay on the root label, the rest goes to the native control
const attrs = useAttrs()
const rest = computed(() => {
  const { class: _c, style: _s, ...r } = attrs
  return r
})
</script>

<template>
  <label class="tg-field" :class="[attrs.class, { 'tg-field-flat': grouped }]" :style="attrs.style as any">
    <span v-if="icon === 'search'" aria-hidden="true">🔍</span>
    <input
      :aria-label="placeholder"
      v-bind="rest"
      :value="modelValue"
      :placeholder="placeholder"
      :type="type ?? (icon === 'search' ? 'search' : 'text')"
      @change="$emit('change')"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
  </label>
</template>
