<script setup lang="ts">
import { computed, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })
defineProps<{ modelValue: string; options: { value: string; label: string }[]; grouped?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [string]; change: [] }>()

// update first, then change: consumers reading the model in @change see the new value
function onChange(e: Event) {
  emit('update:modelValue', (e.target as HTMLSelectElement).value)
  emit('change')
}

// class/style stay on the root label, the rest goes to the native control
const attrs = useAttrs()
const rest = computed(() => {
  const { class: _c, style: _s, ...r } = attrs
  return r
})
</script>

<template>
  <label class="tg-field" :class="[attrs.class, { 'tg-field-flat': grouped }]" :style="attrs.style as any">
    <select v-bind="rest" :value="modelValue" @change="onChange">
      <option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option>
    </select>
    <span aria-hidden="true">⌄</span>
  </label>
</template>
