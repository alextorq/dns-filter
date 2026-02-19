<script setup lang="ts">
import {UButton} from "#components";
import {api, type ExcludeClient} from "~/api";
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
import {getErrorMessage} from "~~/utils/get-error-message";

const toast = useToast()
const {isLoading, createLoadingRequest} = useComponentStatusWithLoading()

const props = defineProps<{
  record: ExcludeClient
}>()

const emit = defineEmits<{
  (e: 'delete', value: ExcludeClient): void
}>()

const deleteClient = async () => {
  try {
    await api.deleteClient(props.record.id)
    emit('delete', props.record)
  } catch (error) {
    const message = getErrorMessage(error)
    toast.add({
      title: 'Error',
      description: message,
      duration: 5000,
      color: 'error',
    })
    console.error('Error deleting client:', error)
  }
}

const fetchWithLoading = createLoadingRequest(deleteClient)
</script>

<template>
  <UButton
    color="red"
    variant="soft"
    icon="i-heroicons-trash"
    :loading="isLoading"
    @click="fetchWithLoading"
  />
</template>

<style scoped>

</style>
