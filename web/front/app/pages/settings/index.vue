<script setup lang="ts">
import {api} from "~/api";
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
import {getErrorMessage} from "~~/utils/get-error-message";
const {isLoading, createLoadingRequest} = useComponentStatusWithLoading()
const toast = useToast()

const getLogLevel = async () => {
  try {
    const data = await api.getLogLevel()
    level.value = data.level
  } catch (error) {
    console.error('Error updating log level:', error)
  }
}

const loadLogLevel = createLoadingRequest(getLogLevel)

const items = ref(['DEBUG', 'INFO', 'WARN', 'ERROR'])
const level = ref('')

const changeLogLevel = async () => {
  try {
    await api.changeLogLevel(level.value)
    toast.add({
      title: 'Success',
      description: 'Log level was updated.',
      duration: 3000,
    })
  } catch (error) {
    console.error('Error updating log level:', error)
    const message = getErrorMessage(error)
    toast.add({
      title: 'Error',
      description: message,
      duration: 5000,
      color: 'error',
    })
  }
}

onMounted(loadLogLevel)
</script>

<template>
  <UContainer>
    <h2>Log Level</h2>
    <div class="w-full space-y-4 pb-4">
      <div class="flex px-4 py-3.5 justify-between border-b border-accented">
        <USelect :loading="isLoading" size="xl" @change="changeLogLevel" v-model="level" :items="items" />
      </div>
    </div>
  </UContainer>
</template>
