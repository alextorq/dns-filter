<script setup lang="ts">
import { api } from "~/api";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

const open = ref(false);

const doClear = async () => {
    try {
        const data = await api.clearDnsCache();
        const cleared = data.cleared ?? 0;
        toast.add({
            title: "DNS cache cleared",
            description:
                cleared === 0
                    ? "Cache was already empty."
                    : `Removed ${cleared} cached ${cleared === 1 ? "entry" : "entries"}.`,
            duration: 4000,
            color: "success",
        });
        open.value = false;
    } catch (error) {
        toast.add({
            title: "Failed to clear cache",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        console.error("Error clearing DNS cache:", error);
    }
};

const confirmClear = createLoadingRequest(doClear);
</script>

<template>
    <UModal v-model:open="open" title="Clear DNS cache">
        <UButton
            size="lg"
            icon="i-lucide-trash-2"
            color="error"
            variant="soft"
            label="Clear DNS cache"
            :loading="isLoading"
        />

        <template #body>
            <p class="text-sm text-muted">
                Drop every entry from the in-memory DNS response cache. The next query for each name
                will go to the upstream resolver, so traffic and latency will temporarily increase
                while the cache rewarms.
            </p>
            <p class="text-sm text-muted mt-2">
                Block lists, allow lists and client overrides are
                <span class="font-medium text-default">not</span>
                affected.
            </p>
        </template>

        <template #footer>
            <div class="flex justify-end gap-2 w-full">
                <UButton
                    color="neutral"
                    variant="ghost"
                    :disabled="isLoading"
                    @click="open = false"
                >
                    Cancel
                </UButton>
                <UButton color="error" :loading="isLoading" @click="confirmClear">
                    Clear cache
                </UButton>
            </div>
        </template>
    </UModal>
</template>
