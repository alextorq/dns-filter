<script setup lang="ts">
import {api} from "~/api";
import Char from "~/domain/cher-by-domains/char.vue";

const status = ref(false)

onMounted(() => {
  const fetchData = async () => {
    try {
      status.value = await api.getFilterStatus()
    } catch (error) {
      console.error('Error fetching data:', error)
    }
  }

  fetchData()
})


const changeStatus = async () => {
  try {
    await api.changeFilterStatus()
  } catch (error) {
    console.error('Error updating status:', error)
  }
}

</script>

<template>
  <div style="height: calc(100vh - var(--ui-header-height));" class="flex justify-center items-center height-100">
    <USwitch
        size="xl"
        @change="changeStatus"
        :model-value="status" label="Status" />
  </div>
</template>
