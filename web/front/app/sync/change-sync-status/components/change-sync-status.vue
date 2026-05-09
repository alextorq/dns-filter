<script setup lang="ts">
import { USwitch } from "#components";
import { api } from "~/api";
import type { DbSource } from "~/api/generated/data-contracts";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

const props = defineProps<{
    record: DbSource;
}>();

const emit = defineEmits<{
    (e: "update", value: DbSource): void;
}>();

const updateActiveStatus = async () => {
    try {
        const res = await api.changeSyncRecordStatus(props.record.id!, !props.record.active);
        if (res) emit("update", res);
    } catch (error) {
        const message = getErrorMessage(error);
        toast.add({
            title: "Error",
            description: message,
            duration: 5000,
            color: "error",
        });
        console.error("Error updating status:", error);
    }
};

const fetchWithLoading = createLoadingRequest(updateActiveStatus);
</script>

<template>
    <USwitch
        size="xl"
        :loading="isLoading"
        class="justify-end"
        :model-value="record.active"
        @update:model-value="fetchWithLoading"
    ></USwitch>
</template>

<style scoped></style>
