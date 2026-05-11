<script setup lang="ts">
import {
    DomainInspectVerdict,
    type DomainInspectInspectResult,
} from "~/api/generated/data-contracts";
import CheckCard from "./check-card.vue";
import VerdictBadge from "./verdict-badge.vue";

defineProps<{
    result: DomainInspectInspectResult;
}>();

// Sorted view is provided by the backend already, but we re-derive it on the
// client so a future request that comes back unsorted still renders sanely.
const verdictHeadline: Record<DomainInspectVerdict, string> = {
    [DomainInspectVerdict.VerdictMalicious]: "Block recommended",
    [DomainInspectVerdict.VerdictSuspicious]: "Review before blocking",
    [DomainInspectVerdict.VerdictClean]: "Looks legitimate",
    [DomainInspectVerdict.VerdictUnknown]: "Not enough signal",
};

const progressColor: Record<DomainInspectVerdict, "error" | "warning" | "success" | "neutral"> = {
    [DomainInspectVerdict.VerdictMalicious]: "error",
    [DomainInspectVerdict.VerdictSuspicious]: "warning",
    [DomainInspectVerdict.VerdictClean]: "success",
    [DomainInspectVerdict.VerdictUnknown]: "neutral",
};
</script>

<template>
    <div class="flex flex-col gap-4">
        <UCard variant="solid">
            <div class="flex flex-wrap items-start gap-4">
                <div class="flex-1 min-w-0">
                    <div class="text-xs text-muted uppercase tracking-wide">Domain</div>
                    <div class="font-mono text-lg break-all">{{ result.domain }}</div>
                </div>
                <div class="flex flex-col items-end gap-2">
                    <VerdictBadge
                        :verdict="result.summary?.verdict ?? DomainInspectVerdict.VerdictUnknown"
                        size="lg"
                    />
                    <div class="text-xs text-muted">
                        {{
                            verdictHeadline[
                                result.summary?.verdict ?? DomainInspectVerdict.VerdictUnknown
                            ]
                        }}
                    </div>
                </div>
            </div>

            <div class="mt-4">
                <div class="flex justify-between text-xs text-muted mb-1">
                    <span>Risk score</span>
                    <span class="tabular-nums">{{ result.summary?.score ?? 0 }} / 100</span>
                </div>
                <UProgress
                    :model-value="result.summary?.score ?? 0"
                    :max="100"
                    :color="
                        progressColor[
                            result.summary?.verdict ?? DomainInspectVerdict.VerdictUnknown
                        ]
                    "
                />
            </div>
        </UCard>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
            <CheckCard v-for="check in result.checks ?? []" :key="check.name" :check="check" />
        </div>
    </div>
</template>
