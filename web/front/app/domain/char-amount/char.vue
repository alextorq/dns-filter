<script setup lang="ts">
import {api} from '~/api';
import {onMounted, ref} from 'vue';

const allAmount = ref(0)

const loadData = async () => {
  return await api.getBlockDomainsAmount();
};

const loadDataAndCreateChart = async () => {
  const data = await loadData();
  allAmount.value = data.amount;
};

onMounted(loadDataAndCreateChart)

</script>

<template>
  {{ allAmount }}
</template>

<style scoped>
canvas {
  width: 400px;
  height: 400px;
}
</style>