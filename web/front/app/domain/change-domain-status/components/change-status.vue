<script setup lang="ts">
import {USwitch} from "#components";
import {api, type DNSRecord} from "~/api";
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
const {isLoading, createLoadingRequest} = useComponentStatusWithLoading()

const props = defineProps<{
  record: DNSRecord
}>()

const emit = defineEmits<{
  (e: 'update', value: DNSRecord): void
}>()

const updateActiveStatus = async () => {
  try {
    const res = await api.changeDnsRecordStatus(props.record.id, !props.record.active)
    emit('update', res)
  } catch (error) {
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