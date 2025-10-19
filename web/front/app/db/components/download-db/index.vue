<script setup lang="ts">
import {api, API_HOST} from "~/api";
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
import {getErrorMessage} from "~~/utils/get-error-message";
import {downloadFile} from "~~/utils/download";
const toast = useToast()

const downloadDb = async () => {
  try {
    downloadFile(API_HOST + '/config/db/download', 'db.sqlite') // Use
    toast.add({
      title: 'Success',
      description: 'Database download initiated.',
      duration: 3000,
    })
  } catch (error) {
    console.error('Error updating log level:', error)
  }
}


</script>

<template>
  <div class="w-full space-y-4 pb-4">
    <div class="flex px-4 py-3.5 justify-between border-b border-accented">
      <UButton  size="xl" @click="downloadDb" >Download Database</UButton>
    </div>
  </div>

</template>
