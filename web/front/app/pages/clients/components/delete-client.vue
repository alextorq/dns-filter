<script setup lang="ts">
import { api } from "~/api";
import type { DbExcludeClient } from "~/api/generated/data-contracts";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

const open = ref(false);

const props = defineProps<{
    record: DbExcludeClient;
}>();

const emit = defineEmits<{
    (e: "delete", value: DbExcludeClient): void;
}>();

const deleteClient = async () => {
    try {
        await api.deleteClient(props.record.id!);
        emit("delete", props.record);
        open.value = false;
    } catch (error) {
        const message = getErrorMessage(error);
        toast.add({
            title: "Error",
            description: message,
            duration: 5000,
            color: "error",
        });
        console.error("Error deleting client:", error);
    }
};

const confirmDelete = createLoadingRequest(deleteClient);
</script>

<template>
    <UModal v-model:open="open" title="Delete client">
        <UButton
            color="error"
            variant="soft"
            icon="i-lucide-trash-2"
            aria-label="Delete client"
            :loading="isLoading"
        />

        <template #body>
            <p class="text-sm text-muted">
                Remove
                <span class="font-mono text-default">{{ record.user_id }}</span>
                from the exclusion list? Filtering will resume for this client.
            </p>
        </template>

        <template #footer>
            <div class="flex justify-end gap-2 w-full">
                <UButton color="neutral" variant="ghost" :disabled="isLoading" @click="open = false">
                    Cancel
                </UButton>
                <UButton color="error" :loading="isLoading" @click="confirmDelete">
                    Delete
                </UButton>
            </div>
        </template>
    </UModal>
</template>
