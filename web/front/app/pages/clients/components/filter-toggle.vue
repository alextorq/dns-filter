<script setup lang="ts">
import { USwitch } from "#components";
import { api } from "~/api";
import type { DbClient } from "~/api/generated/data-contracts";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

const props = defineProps<{
    record: DbClient;
}>();

const emit = defineEmits<{
    (e: "update", value: DbClient): void;
}>();

const toggleFilter = async () => {
    const next = !props.record.filtered;
    try {
        await api.changeClientFilter(props.record.id!, next);
        emit("update", { ...props.record, filtered: next });
    } catch (error) {
        toast.add({
            title: "Error",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        console.error("Error updating filter:", error);
    }
};

const submitToggle = createLoadingRequest(toggleFilter);
</script>

<template>
    <USwitch
        size="xl"
        :loading="isLoading"
        class="justify-end"
        :model-value="record.filtered"
        @update:model-value="submitToggle"
    />
</template>
