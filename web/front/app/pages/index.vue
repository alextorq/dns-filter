<script setup lang="ts">
import { api } from "~/api";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

useHead({
    title: "Filter",
});

const status = ref<boolean | null>(null);

const fetchData = async () => {
    try {
        status.value = (await api.getFilterStatus()) ?? false;
    } catch (error) {
        toast.add({
            title: "Error",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        console.error("Error fetching data:", error);
    }
};

const fetchDataWithLoading = createLoadingRequest(fetchData);

onMounted(fetchDataWithLoading);

const changeStatus = async () => {
    try {
        const next = await api.changeFilterStatus();
        if (typeof next === "boolean") {
            status.value = next;
        }
    } catch (error) {
        toast.add({
            title: "Error",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        console.error("Error updating status:", error);
    }
};

const changeStatusWithLoading = createLoadingRequest(changeStatus);
</script>

<template>
    <div
        style="height: calc(100vh - var(--ui-header-height))"
        class="flex flex-col items-center justify-center gap-6 px-4 text-center"
    >
        <div class="space-y-2">
            <h1 class="text-2xl font-semibold text-highlighted">DNS Filter</h1>
            <p class="text-sm text-muted max-w-md">
                Toggle the global filter. When enabled, blocked domains are answered with
                <span class="font-mono">NXDOMAIN</span>; when disabled, every query is forwarded
                upstream untouched.
            </p>
        </div>

        <USwitch
            v-if="status !== null"
            size="xl"
            :loading="isLoading"
            :model-value="status"
            :label="status ? 'Active' : 'Disabled'"
            @update:model-value="changeStatusWithLoading"
        />
        <USkeleton v-else class="h-6 w-32" />
    </div>
</template>
