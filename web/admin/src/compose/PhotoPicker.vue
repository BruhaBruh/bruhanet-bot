<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { ApiError, upload } from '../api'
import { TgButton } from '../ui'
import { checkPhoto } from './photo'

// v-model — file_id загруженного фото
const props = defineProps<{ modelValue?: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [string | undefined]; uploading: [boolean] }>()

const url = ref('')
const busy = ref(false)
const error = ref('')
const input = ref<HTMLInputElement>()
watch(busy, (v) => emit('uploading', v))

function setUrl(v: string) {
  if (url.value) URL.revokeObjectURL(url.value)
  url.value = v
}
// родитель сбросил фото (после отправки) — убрать превью
// gen меняется при сбросе/удалении: результат загрузки, начатой до этого, игнорируется
let gen = 0
watch(() => props.modelValue, (v) => {
  if (v) return
  gen++
  setUrl('')
})
onBeforeUnmount(() => setUrl(''))

async function pick(e: Event) {
  const el = e.target as HTMLInputElement
  const f = el.files?.[0]
  el.value = ''
  if (!f) return
  error.value = checkPhoto(f) ?? ''
  if (error.value) return
  busy.value = true
  const g = gen
  try {
    const { photo } = await upload<{ photo: string }>('/api/photos', f)
    if (g !== gen) return
    setUrl(URL.createObjectURL(f))
    emit('update:modelValue', photo)
  } catch (err) {
    if (g !== gen) return
    error.value = err instanceof ApiError && err.body.message ? err.body.message : 'Не удалось загрузить фото'
  } finally {
    busy.value = false
  }
}

function remove() {
  gen++
  setUrl('')
  emit('update:modelValue', undefined)
}
</script>

<template>
  <div class="photo">
    <input ref="input" type="file" accept="image/jpeg,image/png,image/webp" hidden @change="pick" />
    <template v-if="modelValue">
      <img v-if="url" :src="url" alt="Фото" />
      <TgButton mode="plain" size="s" :disabled="disabled" @click="remove">Удалить фото</TgButton>
    </template>
    <TgButton v-else mode="bezeled" size="s" :loading="busy" :disabled="disabled" @click="input?.click()">🖼 Добавить фото</TgButton>
    <span v-if="busy" class="tg-visually-hidden" role="status">Загрузка…</span>
    <p v-if="error" class="tg-err" role="alert">{{ error }}</p>
  </div>
</template>

<style scoped>
.photo { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
img { max-width: 160px; max-height: 120px; border-radius: 8px; object-fit: cover; }
.tg-err { margin: 0; width: 100%; }
</style>
