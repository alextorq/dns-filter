<script setup lang="ts">
import Chart from "chart.js/auto";
import { api } from "~/api";
import type { DbDomainCount } from "~/api/generated/data-contracts";
import { onMounted, ref } from "vue";
import type { ChartData, ChartDataset } from "chart.js";
const chartRef = ref<HTMLCanvasElement | null>(null);

const adaptData = (data: DbDomainCount[]): ChartData<"doughnut", number[], string> => {
    const sordedData = data.sort((a, b) => (b.count ?? 0) - (a.count ?? 0));
    data = sordedData.slice(0, 10); // берем топ 10

    const dataSet: ChartDataset<"doughnut", number[]> = {
        label: "Blocked Domains",
        data: data.map((item) => item.count ?? 0),
        borderWidth: 1,
    };

    return {
        labels: data.map((item) => item.domain ?? ""),
        datasets: [dataSet],
    };
};

const createChart = (data: DbDomainCount[]) => {
    if (chartRef.value) {
        new Chart(chartRef.value, {
            type: "doughnut",
            data: adaptData(data),
            options: {
                responsive: false, // <-- отключаем
                maintainAspectRatio: false, // можно убрать привязку к соотношению сторон
                plugins: {
                    legend: {
                        display: false,
                    },
                },
            },
        });
    }
};

const loadData = async () => {
    return await api.getBlockDomainsGroups();
};

const loadDataAndCreateChart = async () => {
    const data = await loadData();
    createChart(data.groups ?? []);
};

onMounted(loadDataAndCreateChart);
</script>

<template>
    <canvas ref="chartRef" width="400" height="400"></canvas>
</template>

<style scoped>
canvas {
    width: 400px;
    height: 400px;
}
</style>
