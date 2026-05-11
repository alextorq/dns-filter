<script setup lang="ts">
import { computed } from "vue";
import {
    DomainInspectVerdict,
    type DomainInspectCheckResult,
} from "~/api/generated/data-contracts";
import { useInspectRegistry } from "~~/composables/use-inspect-registry";

const props = defineProps<{ domain: string }>();
const { get } = useInspectRegistry();

const entry = computed(() => get(props.domain));

const verdictColor: Record<DomainInspectVerdict, "error" | "warning" | "success" | "neutral"> = {
    [DomainInspectVerdict.VerdictMalicious]: "error",
    [DomainInspectVerdict.VerdictSuspicious]: "warning",
    [DomainInspectVerdict.VerdictClean]: "success",
    [DomainInspectVerdict.VerdictUnknown]: "neutral",
};

const verdictLabel: Record<DomainInspectVerdict, string> = {
    [DomainInspectVerdict.VerdictMalicious]: "Malicious",
    [DomainInspectVerdict.VerdictSuspicious]: "Suspicious",
    [DomainInspectVerdict.VerdictClean]: "Clean",
    [DomainInspectVerdict.VerdictUnknown]: "Unknown",
};

const formatCheckName = (name?: string): string => (name ?? "").replace(/[-_]/g, " ");

const isFlagged = (c: DomainInspectCheckResult): boolean =>
    c.verdict === DomainInspectVerdict.VerdictMalicious ||
    c.verdict === DomainInspectVerdict.VerdictSuspicious;

const overallVerdict = computed<DomainInspectVerdict>(
    () => entry.value?.result?.summary?.verdict ?? DomainInspectVerdict.VerdictUnknown,
);

const flaggedChecks = computed(() => (entry.value?.result?.checks ?? []).filter(isFlagged));
</script>

<template>
    <div v-if="!entry || entry.status === 'idle'" class="text-xs text-muted">—</div>

    <div
        v-else-if="entry.status === 'loading'"
        class="flex items-center gap-1.5 text-xs text-muted"
    >
        <UIcon name="i-lucide-loader-2" class="size-3.5 animate-spin" />
        <span>Scanning…</span>
    </div>

    <UTooltip v-else-if="entry.status === 'error'" :text="entry.error || 'Scan failed'">
        <UBadge color="neutral" variant="subtle" size="sm" icon="i-lucide-circle-alert">
            Scan failed
        </UBadge>
    </UTooltip>

    <div v-else class="flex flex-wrap items-center gap-1">
        <UBadge :color="verdictColor[overallVerdict]" variant="subtle" size="sm">
            {{ verdictLabel[overallVerdict] }}
        </UBadge>
        <UBadge
            v-for="c in flaggedChecks"
            :key="c.name"
            :color="verdictColor[c.verdict ?? DomainInspectVerdict.VerdictUnknown]"
            variant="solid"
            size="sm"
            class="font-mono"
        >
            {{ formatCheckName(c.name) }}
        </UBadge>
    </div>
</template>
