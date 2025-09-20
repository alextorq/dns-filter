<script setup lang="ts">
const items = ref([
  {
    label: 'Domains',
    icon: 'i-lucide-book-open',
    to: '/domains',
  },
])
</script>

<template>
  <UApp>
    <UHeader title="DNS FILTER">
      <UNavigationMenu :items="items" />
    </UHeader>
    <UMain>
      <NuxtLayout>
        <NuxtPage />
      </NuxtLayout>
    </UMain>
  </UApp>
</template>


<style>
@import "tailwindcss";
@import "@nuxt/ui";
</style>