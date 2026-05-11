<script setup lang="ts">
import InspectResultPanel from "~/domain/inspect/components/inspect-result-panel.vue";
import { useInspectDomain } from "~~/composables/use-inspect-domain";

useHead({ title: "Inspect" });

const domain = ref("");
const { result, isLoading, errorMessage, run, reset } = useInspectDomain();

const onSubmit = () => run(domain.value);

onScopeDispose(() => reset());
</script>

<template>
    <UContainer class="py-6">
        <div class="flex flex-col gap-2 mb-6">
            <h1 class="text-2xl font-semibold">Inspect domain</h1>
            <p class="text-sm text-muted">
                Run reputation, registration-age, certificate-transparency, and third-party scanner
                checks against a domain before adding it to the block list.
            </p>
        </div>

        <UForm :state="{ domain }" class="mb-6" @submit="onSubmit">
            <div class="flex flex-wrap gap-3 items-start">
                <UFormField name="domain" class="flex-1 min-w-[260px]" :error="errorMessage">
                    <UInput
                        v-model="domain"
                        size="xl"
                        class="w-full"
                        placeholder="example.com"
                        icon="i-lucide-search"
                        autofocus
                        :disabled="isLoading"
                    />
                </UFormField>
                <UButton
                    type="submit"
                    size="xl"
                    label="Inspect"
                    icon="i-lucide-radar"
                    :loading="isLoading"
                    :disabled="!domain.trim()"
                />
            </div>
        </UForm>

        <UAlert
            v-if="errorMessage && !isLoading && !result"
            color="error"
            variant="subtle"
            icon="i-lucide-circle-alert"
            title="Inspection failed"
            :description="errorMessage"
        />

        <div v-if="isLoading" class="flex flex-col gap-3">
            <USkeleton class="h-28 w-full" />
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                <USkeleton v-for="i in 4" :key="i" class="h-32 w-full" />
            </div>
        </div>

        <InspectResultPanel v-else-if="result" :result="result" />

        <div v-else-if="!errorMessage" class="text-center text-muted py-12">
            <UIcon name="i-lucide-search" class="text-4xl mb-2" />
            <p>Enter a domain above to start an inspection.</p>
        </div>
    </UContainer>
</template>
