<script setup lang="ts">
import {USwitch} from "#components";
import {api, type SyncRecord} from "~/api";
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
import {getErrorMessage} from "~~/utils/get-error-message";

const toast = useToast()
const {isLoading, createLoadingRequest} = useComponentStatusWithLoading()

const props = defineProps<{
  record: SyncRecord
}>()

const emit = defineEmits<{
  (e: 'update', value: SyncRecord): void
}>()

const updateActiveStatus = async () => {
  try {
    const res = await api.changeSyncRecordStatus(props.record.id, !props.record.active)
    emit('update', res)
  } catch (error) {
    const message = getErrorMessage(error)
    toast.add({
      title: 'Error',
      description: message,
      duration: 5000,
      color: 'error',
    })
    console.error('Error updating status:', error)
  }
}

const fetchWithLoading = createLoadingRequest(updateActiveStatus)
</script>

<template>
  <USwitch
    size="xl"
    :loading="isLoading"
    @update:model-value="fetchWithLoading"
    class="justify-end"
    :model-value="record.active"
  ></USwitch>
</template>

<style scoped>

</style>
