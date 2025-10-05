<script setup lang="ts">
import {api} from "~/api";
import {useComponentStatusWithLoading} from "~~/composables/use-component-status-with-loading";
import {USwitch} from "#components";
const {isLoading, createLoadingRequest} = useComponentStatusWithLoading()

const status = ref(true)

const fetchData = async () => {
  try {
    status.value = await api.getFilterStatus()
  } catch (error) {
    console.error('Error fetching data:', error)
  }
}

const fetchDataWithLoading = createLoadingRequest(fetchData)

onMounted(fetchDataWithLoading)

const changeStatus = async () => {
  try {
    await api.changeFilterStatus()
  } catch (error) {
    console.error('Error updating status:', error)
  }
}

const changeStatusWithLoading = createLoadingRequest(changeStatus)

</script>

<template>
  <div style="height: calc(100vh - var(--ui-header-height));"
       class="flex justify-center items-center height-100">
    <USwitch
        :loading="isLoading"
        size="xl"
        @change="changeStatusWithLoading"
        :model-value="status"
        label="Status" />
  </div>
</template>
