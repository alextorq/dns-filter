<script setup lang="ts">
import {
    DomainInspectCheckStatus,
    type DomainInspectCheckResult,
} from "~/api/generated/data-contracts";
import VerdictBadge from "./verdict-badge.vue";

const props = defineProps<{
    check: DomainInspectCheckResult;
}>();

const statusColor: Record<DomainInspectCheckStatus, "success" | "neutral" | "error" | "warning"> = {
    [DomainInspectCheckStatus.StatusOK]: "success",
    [DomainInspectCheckStatus.StatusError]: "error",
    [DomainInspectCheckStatus.StatusSkipped]: "neutral",
    [DomainInspectCheckStatus.StatusTimeout]: "warning",
};

const statusLabel: Record<DomainInspectCheckStatus, string> = {
    [DomainInspectCheckStatus.StatusOK]: "OK",
    [DomainInspectCheckStatus.StatusError]: "Error",
    [DomainInspectCheckStatus.StatusSkipped]: "Skipped",
    [DomainInspectCheckStatus.StatusTimeout]: "Timeout",
};

// detailEntries pre-stringifies non-primitive values once — the template
// re-evaluates expressions on every render, so JSON.stringify in the template
// would re-allocate on every keystroke / scroll.
const detailEntries = computed(() => {
    const details = props.check.details ?? {};
    return Object.entries(details).map(([key, value]) => ({
        key,
        value: formatValue(value),
    }));
});

function formatValue(value: unknown): string {
    if (value === null || value === undefined) return "—";
    if (typeof value === "string") return value;
    if (typeof value === "number" || typeof value === "boolean") return String(value);
    try {
        return JSON.stringify(value);
    } catch {
        return String(value);
    }
}

const status = computed(() => props.check.status ?? DomainInspectCheckStatus.StatusError);
const hasDetails = computed(() => detailEntries.value.length > 0);
const hasError = computed(() => Boolean(props.check.error));
</script>

<template>
    <UCard :ui="{ body: 'p-4 sm:p-4', header: 'p-4 sm:p-4' }" variant="subtle">
        <template #header>
            <div class="flex items-center gap-3 flex-wrap">
                <span class="font-mono text-sm font-semibold">{{ check.name }}</span>
                <UBadge :color="statusColor[status]" variant="subtle" size="sm">
                    {{ statusLabel[status] }}
                </UBadge>
                <VerdictBadge v-if="check.verdict" :verdict="check.verdict" size="sm" />
                <span
                    v-if="check.duration_ms !== undefined"
                    class="ml-auto text-xs text-muted tabular-nums"
                >
                    {{ check.duration_ms }} ms
                </span>
            </div>
        </template>

        <div v-if="hasError" class="text-sm text-error mb-2">
            {{ check.error }}
        </div>

        <dl v-if="hasDetails" class="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-sm">
            <template v-for="entry in detailEntries" :key="entry.key">
                <dt class="text-muted font-mono">{{ entry.key }}</dt>
                <dd class="font-mono break-all">{{ entry.value }}</dd>
            </template>
        </dl>

        <p v-else-if="!hasError" class="text-sm text-muted">No details</p>
    </UCard>
</template>
