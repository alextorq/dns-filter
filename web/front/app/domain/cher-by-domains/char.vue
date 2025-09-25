<script setup lang="ts">
import Chart from 'chart.js/auto';
import {api, type DomainBlockWithCount} from '~/api';
import {onMounted, ref} from 'vue';
import type { ChartData, ChartDataset } from 'chart.js';
const chartRef = ref<HTMLCanvasElement|null>(null);

const adaptData = (data: DomainBlockWithCount[]): ChartData<'doughnut', number[], string> => {
  const sordedData = data.sort((a, b) => b.Count - a.Count);
  data = sordedData.slice(0, 10); // берем топ 10

  const dataSet : ChartDataset<'doughnut', number[]> = {
    label: 'Blocked Domains',
    data: data.map(item => item.Count),
    borderWidth: 1
  }

  return {
    labels: data.map(item => item.Domain),
    datasets: [dataSet]
  };
};

const createChart = (data: any) => {
  if (chartRef.value) {
    const chart = new Chart(chartRef.value, {
      type: 'doughnut',
      data: adaptData(data),
      options: {
        responsive: false, // <-- отключаем
        maintainAspectRatio: false, // можно убрать привязку к соотношению сторон
        plugins: {
          legend: {
            display: false
          }
        }
      }
    });
  }
};

const loadData = async () => {
  return await api.getBlockDomainsGroups();
};


const loadDataAndCreateChart = async () => {
  const data = await loadData();
  createChart(data.groups);
};

onMounted(loadDataAndCreateChart)

</script>

<template>
  <canvas width="400" height="400" ref="chartRef"></canvas>
</template>

<style scoped>
canvas {
  width: 400px;
  height: 400px;
}
</style>