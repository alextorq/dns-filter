<script setup lang="ts">
import {api} from "~/api";

const items = ref(['DEBUG', 'INFO', 'WARN', 'ERROR'])
const level = ref('')

const changeLogLevel = async () => {
  try {
    await api.changeLogLevel(level.value)
  } catch (error) {
    console.error('Error updating log level:', error)
  }
}

const getLogLevel = async () => {
  try {
    const data = await api.getLogLevel()
    level.value = data.level
  } catch (error) {
    console.error('Error updating log level:', error)
  }
}

onMounted(getLogLevel)


</script>

<template>
  <UContainer>
    <h2>Log Level</h2>
    <div class="w-full space-y-4 pb-4">
      <div class="flex px-4 py-3.5 justify-between border-b border-accented">
        <USelect size="xl" @change="changeLogLevel" v-model="level" :items="items" />
      </div>
    </div>
  </UContainer>
</template>
