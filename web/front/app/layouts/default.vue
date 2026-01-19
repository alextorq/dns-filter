<script setup lang="ts">
const items = ref([
  {
    label: 'Domains',
    icon: 'i-lucide-book-open',
    to: '/domains',
  },
  {
    label: 'Sources',
    icon: 'i-lucide-refresh-cw',
    to: '/sources',
  },
  {
    label: 'Statistic',
    icon: 'i-lucide-pie-chart',
    to: '/statistic',
  },
  {
    label: 'Suggest',
    icon: 'i-lucide-lightbulb',
    to: '/suggest',
  },
  {
    label: 'Settings',
    icon: 'i-lucide-settings',
    to: '/settings',
  }
])
</script>

<template>
  <UApp>
    <UHeader title="DNS FILTER">
      <UNavigationMenu :items="items" />
      <template #body>
        <UNavigationMenu :items="items" />
      </template>
    </UHeader>
    <UMain>
       <NuxtPage />
    </UMain>
  </UApp>
</template>


<style>
@import "tailwindcss";
@import "@nuxt/ui";
</style>