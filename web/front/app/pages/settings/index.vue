<script setup lang="ts">
import type { TableColumn } from '@nuxt/ui'
import {api, type DNSRecord} from "~/api";

const items = ref(['Backlog', 'Todo', 'In Progress', 'Done'])
const value = ref('Backlog')

const changeLogLevel = async (record: DNSRecord) => {
  try {
    await api.changeLogLevel(record.ID)
    record.LogLevel = !record.LogLevel
  } catch (error) {
    console.error('Error updating log level:', error)
  }
}


</script>

<template>
  <UContainer>
    <div class="w-full space-y-4 pb-4">
      <div class="flex px-4 py-3.5 justify-between border-b border-accented">
        <USelect v-model="value" :items="items" />
      </div>
    </div>
  </UContainer>
</template>
