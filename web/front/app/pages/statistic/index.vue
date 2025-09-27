<script setup lang="ts">
import Char from "~/domain/char-by-domains/char.vue";
import AllAmount from "~/domain/char-amount/char.vue";
</script>

<template>
  <div class="statistic-page flex flex-col items-center justify-center min-h-screen py-8">
    <div class="statistic-container flex flex-col md:flex-row gap-8 w-full max-w-4xl">
      <!-- Total Blocked Domains Block -->
      <div class="stat-block flex-1 rounded-xl shadow-lg p-8 flex flex-col items-center justify-center">
        <h2 class="text-2xl font-bold mb-4 ">Total Blocked Domains</h2>
        <AllAmount/>
      </div>
      <!-- Blocked Domains Statistics Block -->
      <div class="stat-block flex-1 rounded-xl shadow-lg p-8 flex flex-col items-center justify-center">
        <h2 class="text-2xl font-bold mb-4 ">Blocked Domains Group</h2>
        <div class="w-full flex justify-center">
          <Char/>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.statistic-page {
  min-height: 100vh;
  color: wheat;
}
.statistic-container {
  width: 100%;
  max-width: 900px;
  margin: 0 auto;
}
.stat-block {
  min-width: 280px;
  min-height: 320px;
  background: inherit;
  transition: box-shadow 0.2s;
}
.stat-block:hover {
  box-shadow: 0 8px 32px rgba(0,0,0,0.12);
}
@media (max-width: 768px) {
  .statistic-container {
    flex-direction: column;
    gap: 2rem;
  }
  .stat-block {
    min-width: unset;
    width: 100%;
  }
}
</style>
