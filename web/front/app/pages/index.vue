<script setup lang="ts">
import { api } from "~/api";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

useHead({
    title: "Filter",
});

const PAUSE_OPTIONS: number[] = [5, 10, 15, 30];

const status = ref<boolean | null>(null);
const pausedUntil = ref<number>(0);
const nowUnix = ref<number>(Math.floor(Date.now() / 1000));
const selectedMinutes = ref<number>(PAUSE_OPTIONS[0]!);

let tickHandle: ReturnType<typeof setInterval> | null = null;

const isPaused = computed(() => pausedUntil.value > nowUnix.value);

const secondsLeft = computed(() => Math.max(0, pausedUntil.value - nowUnix.value));

const remainingLabel = computed(() => {
    const s = secondsLeft.value;
    const mm = String(Math.floor(s / 60)).padStart(2, "0");
    const ss = String(s % 60).padStart(2, "0");
    return `${mm}:${ss}`;
});

const startTicker = () => {
    if (tickHandle !== null) return;
    tickHandle = setInterval(() => {
        nowUnix.value = Math.floor(Date.now() / 1000);
        if (!isPaused.value && pausedUntil.value !== 0) {
            // The deadline just passed locally; verify with the server in case
            // someone else extended/cancelled the pause meanwhile.
            pausedUntil.value = 0;
            void fetchData();
        }
    }, 1000);
};

const stopTicker = () => {
    if (tickHandle !== null) {
        clearInterval(tickHandle);
        tickHandle = null;
    }
};

const applyResponse = (data: { status?: boolean; paused_until?: number }) => {
    status.value = data.status ?? false;
    pausedUntil.value = data.paused_until ?? 0;
    nowUnix.value = Math.floor(Date.now() / 1000);
    if (isPaused.value) startTicker();
    else stopTicker();
};

const showError = (error: unknown, title = "Error") => {
    toast.add({
        title,
        description: getErrorMessage(error),
        duration: 5000,
        color: "error",
    });
    console.error(title, error);
};

const fetchData = async () => {
    try {
        applyResponse(await api.getFilterStatus());
    } catch (error) {
        showError(error, "Failed to load filter status");
    }
};

const fetchDataWithLoading = createLoadingRequest(fetchData);

onMounted(fetchDataWithLoading);
onBeforeUnmount(stopTicker);

const changeStatus = async () => {
    try {
        applyResponse(await api.changeFilterStatus());
    } catch (error) {
        showError(error, "Failed to change filter status");
    }
};

const pauseFilter = async () => {
    try {
        applyResponse(await api.pauseFilter(selectedMinutes.value));
    } catch (error) {
        showError(error, "Failed to pause filter");
    }
};

const resumeFilter = async () => {
    try {
        applyResponse(await api.resumeFilter());
    } catch (error) {
        showError(error, "Failed to resume filter");
    }
};

const changeStatusWithLoading = createLoadingRequest(changeStatus);
const pauseFilterWithLoading = createLoadingRequest(pauseFilter);
const resumeFilterWithLoading = createLoadingRequest(resumeFilter);

const pauseSelectItems = PAUSE_OPTIONS.map((m) => ({
    label: `${m} min`,
    value: m,
}));
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

        <div
            v-if="status"
            class="flex flex-col items-center gap-3 border-t border-default pt-6 w-full max-w-xs"
        >
            <template v-if="isPaused">
                <p class="text-sm text-muted">Filter is paused. Resuming in</p>
                <p class="font-mono text-3xl text-highlighted tabular-nums">
                    {{ remainingLabel }}
                </p>
                <UButton
                    color="primary"
                    variant="solid"
                    :loading="isLoading"
                    @click="resumeFilterWithLoading"
                >
                    Resume now
                </UButton>
            </template>
            <template v-else>
                <p class="text-sm text-muted">Pause the filter temporarily</p>
                <USelect
                    v-model="selectedMinutes"
                    :items="pauseSelectItems"
                    value-key="value"
                    class="w-32"
                />
                <UButton
                    color="warning"
                    variant="solid"
                    :loading="isLoading"
                    @click="pauseFilterWithLoading"
                >
                    Pause
                </UButton>
            </template>
        </div>
    </div>
</template>
