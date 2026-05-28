<script setup lang="ts">
import DownloadDb from "~/db/components/download-db/index.vue";
import ClearDnsCache from "~/dns-cache/components/clear-cache/index.vue";
import RuntimeSettings from "./components/runtime-settings.vue";

useHead({
    title: "Settings",
});
</script>

<template>
    <UContainer class="py-8 space-y-6">
        <header class="space-y-1">
            <h1 class="text-2xl font-semibold text-highlighted">Settings</h1>
            <p class="text-sm text-muted">
                Runtime configuration grouped by feature. Persisted in the database — changes apply
                immediately and survive a restart.
            </p>
        </header>

        <RuntimeSettings />

        <UCard>
            <template #header>
                <div class="space-y-1">
                    <h2 class="text-base font-semibold text-highlighted">DNS cache</h2>
                    <p class="text-sm text-muted">
                        Flush the in-memory response cache. Use this after changing upstream DNS
                        records that have a long TTL.
                    </p>
                </div>
            </template>

            <ClearDnsCache />
        </UCard>

        <UCard>
            <template #header>
                <div class="space-y-1">
                    <h2 class="text-base font-semibold text-highlighted">Database</h2>
                    <p class="text-sm text-muted">
                        Export the SQLite database used by the filter (block lists, events,
                        clients).
                    </p>
                </div>
            </template>

            <DownloadDb />
        </UCard>
    </UContainer>
</template>
