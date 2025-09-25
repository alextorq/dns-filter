<script setup lang="ts">
import Char from "~/domain/cher-by-domains/char.vue";
</script>

<template>
  <div style="height: calc(100vh - var(--ui-header-height));" class="flex justify-center items-center height-100">

    <div>
      <h1 class="text-2xl font-bold mb-4 text-center">Blocked Domains Statistics</h1>
      <div class="flex justify-center">
        <Char></Char>
      </div>
    </div>
  </div>
</template>
