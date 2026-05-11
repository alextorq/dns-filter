<script setup lang="ts">
import InspectResultPanel from "./inspect-result-panel.vue";
import { useInspectDomain } from "~~/composables/use-inspect-domain";
import { useToggle } from "~~/composables/use-toggle";

// onBlock is passed by parents that own a domain-blocking action (e.g. the
// suggest-to-block page). When provided the drawer renders an extra footer
// button that triggers the action and closes itself — so the user does not
// have to switch contexts to apply the verdict they just reviewed.
const props = defineProps<{
    domain: string;
    onBlock?: () => Promise<void> | void;
}>();

const { isActive, openHandler, closeHandler } = useToggle();
const { result, isLoading, errorMessage, run, reset } = useInspectDomain();

const open = async () => {
    openHandler();
    if (props.domain) {
        await run(props.domain);
    }
};

const onClose = () => {
    closeHandler();
    reset();
};

const onBlockClick = async () => {
    if (props.onBlock) {
        await props.onBlock();
        onClose();
    }
};
</script>

<template>
    <UButton
        size="sm"
        color="neutral"
        variant="ghost"
        icon="i-lucide-radar"
        :title="`Inspect ${domain}`"
        aria-label="Inspect domain"
        @click="open"
    />

    <UDrawer v-model:open="isActive" direction="right" :handle="false" @close="onClose">
        <template #header>
            <div class="flex items-center gap-2">
                <UIcon name="i-lucide-radar" class="text-primary" />
                <h2 class="text-lg font-semibold">Inspect</h2>
                <code class="font-mono text-sm text-muted truncate">{{ domain }}</code>
            </div>
        </template>

        <template #body>
            <div class="w-full max-w-3xl">
                <UAlert
                    v-if="errorMessage && !isLoading && !result"
                    color="error"
                    variant="subtle"
                    icon="i-lucide-circle-alert"
                    title="Inspection failed"
                    :description="errorMessage"
                    class="mb-4"
                />

                <div v-if="isLoading" class="flex flex-col gap-3">
                    <USkeleton class="h-28 w-full" />
                    <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                        <USkeleton v-for="i in 4" :key="i" class="h-32 w-full" />
                    </div>
                </div>

                <InspectResultPanel v-else-if="result" :result="result" />
            </div>
        </template>

        <template #footer>
            <div class="flex justify-end gap-2 w-full">
                <UButton
                    size="lg"
                    color="neutral"
                    variant="ghost"
                    :disabled="isLoading"
                    @click="onClose"
                >
                    Close
                </UButton>
                <UButton
                    v-if="onBlock"
                    size="lg"
                    color="primary"
                    icon="i-lucide-shield-x"
                    :disabled="isLoading"
                    @click="onBlockClick"
                >
                    Block domain
                </UButton>
            </div>
        </template>
    </UDrawer>
</template>
