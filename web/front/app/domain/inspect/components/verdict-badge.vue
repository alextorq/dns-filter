<script setup lang="ts">
import { DomainInspectVerdict } from "~/api/generated/data-contracts";

const props = defineProps<{
    verdict?: DomainInspectVerdict;
    size?: "sm" | "md" | "lg";
}>();

// Colors map deliberately reuses Nuxt UI semantic tokens so the badge stays
// in sync with the rest of the app (status messages, toasts, etc).
const colorByVerdict: Record<DomainInspectVerdict, "error" | "warning" | "success" | "neutral"> = {
    [DomainInspectVerdict.VerdictMalicious]: "error",
    [DomainInspectVerdict.VerdictSuspicious]: "warning",
    [DomainInspectVerdict.VerdictClean]: "success",
    [DomainInspectVerdict.VerdictUnknown]: "neutral",
};

const labelByVerdict: Record<DomainInspectVerdict, string> = {
    [DomainInspectVerdict.VerdictMalicious]: "Malicious",
    [DomainInspectVerdict.VerdictSuspicious]: "Suspicious",
    [DomainInspectVerdict.VerdictClean]: "Clean",
    [DomainInspectVerdict.VerdictUnknown]: "Unknown",
};

const color = computed(() => (props.verdict ? colorByVerdict[props.verdict] : "neutral"));
const label = computed(() => (props.verdict ? labelByVerdict[props.verdict] : "Unknown"));
</script>

<template>
    <UBadge :color="color" :size="size ?? 'md'" variant="subtle">
        {{ label }}
    </UBadge>
</template>
