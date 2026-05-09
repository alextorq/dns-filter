<script setup lang="ts">
import { api } from "~/api";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import DownloadDb from "~/db/components/download-db/index.vue";
import { getErrorMessage } from "~~/utils/get-error-message";

const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();
const toast = useToast();

useHead({
    title: "Settings",
});

const items = ["DEBUG", "INFO", "WARN", "ERROR"];
const level = ref<string | null>(null);

const getLogLevel = async () => {
    try {
        const data = await api.getLogLevel();
        level.value = data.level ?? null;
    } catch (error) {
        toast.add({
            title: "Error",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        console.error("Error loading log level:", error);
    }
};

const loadLogLevel = createLoadingRequest(getLogLevel);

const changeLogLevel = async () => {
    if (!level.value) return;
    try {
        await api.changeLogLevel(level.value);
        toast.add({
            title: "Saved",
            description: `Log level set to ${level.value}.`,
            duration: 3000,
        });
    } catch (error) {
        toast.add({
            title: "Error",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        console.error("Error updating log level:", error);
    }
};

onMounted(loadLogLevel);
</script>

<template>
    <UContainer class="py-8 space-y-6">
        <header class="space-y-1">
            <h1 class="text-2xl font-semibold text-highlighted">Settings</h1>
            <p class="text-sm text-muted">Runtime configuration for the resolver.</p>
        </header>

        <UCard>
            <template #header>
                <div class="space-y-1">
                    <h2 class="text-base font-semibold text-highlighted">Logging</h2>
                    <p class="text-sm text-muted">
                        Verbosity of the server log stream. Changes apply immediately.
                    </p>
                </div>
            </template>

            <UFormField label="Log level" name="level">
                <USelect
                    v-model="level!"
                    :loading="isLoading"
                    :disabled="isLoading || level === null"
                    size="lg"
                    class="max-w-xs"
                    :items="items"
                    @update:model-value="changeLogLevel"
                />
            </UFormField>
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
