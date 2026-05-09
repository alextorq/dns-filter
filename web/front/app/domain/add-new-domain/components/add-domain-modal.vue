<script setup lang="ts">
import { api } from "~/api";
import { useToggle } from "~~/composables/use-toggle";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();
const { isActive, closeHandler, openHandler } = useToggle();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

const touched = ref(false);
const validationError = ref("");

const state = reactive({
    domain: "",
});

const normalize = (raw: string) => {
    const trimmed = raw.trim().toLowerCase();
    if (!trimmed) return "";
    return trimmed.endsWith(".") ? trimmed : `${trimmed}.`;
};

const validate = (): string | null => {
    const fqdn = normalize(state.domain);
    if (!fqdn) return "Domain is required.";
    if (fqdn.length > 253) return "Domain must not exceed 253 characters.";

    const labels = fqdn.slice(0, -1).split(".");
    for (const label of labels) {
        if (label.length < 1) return "Each label must be at least 1 character.";
        if (label.length > 63) return "Each label must not exceed 63 characters.";
        if (!/^[a-z0-9-]+$/.test(label)) {
            return "Labels may contain only letters, digits, and hyphens.";
        }
        if (label.startsWith("-") || label.endsWith("-")) {
            return "Labels must not start or end with a hyphen.";
        }
    }
    return null;
};

const runValidation = () => {
    validationError.value = validate() ?? "";
    return !validationError.value;
};

watch(
    () => state.domain,
    () => {
        if (touched.value) runValidation();
    },
);

const reset = () => {
    state.domain = "";
    validationError.value = "";
    touched.value = false;
};

const onClose = () => {
    closeHandler();
    reset();
};

const onSubmit = async () => {
    touched.value = true;
    if (!runValidation()) return;
    await api.createDomain(normalize(state.domain));
    toast.add({
        title: "Success",
        description: "New domain was added.",
        duration: 3000,
    });
    onClose();
};

const submitWithLoading = createLoadingRequest(async () => {
    try {
        await onSubmit();
    } catch (e) {
        toast.add({
            title: "Error",
            description: getErrorMessage(e),
            duration: 5000,
            color: "error",
        });
        console.error("Error", e);
        throw e;
    }
});
</script>

<template>
    <UDrawer v-model:open="isActive" direction="right" @close="reset">
        <UButton size="xl" label="Add domain" icon="i-lucide-plus" @click="openHandler" />

        <template #header>
            <h1 class="text-lg font-semibold">Add Domain</h1>
        </template>

        <template #body>
            <UForm
                id="add-domain-form"
                :state="state"
                class="w-full max-w-xl"
                @submit="submitWithLoading"
            >
                <UFormField
                    label="Domain"
                    name="domain"
                    required
                    :error="validationError"
                    help="Trailing dot is optional — will be added automatically."
                >
                    <UInput
                        v-model="state.domain"
                        size="xl"
                        class="w-full"
                        placeholder="ads.example.com"
                        autofocus
                        :disabled="isLoading"
                        @blur="touched = true"
                    />
                </UFormField>
            </UForm>
        </template>

        <template #footer>
            <div class="flex justify-end gap-2 w-full">
                <UButton
                    size="xl"
                    color="neutral"
                    variant="ghost"
                    :disabled="isLoading"
                    @click="onClose"
                >
                    Cancel
                </UButton>
                <UButton
                    size="xl"
                    label="Add domain"
                    type="submit"
                    form="add-domain-form"
                    :loading="isLoading"
                />
            </div>
        </template>
    </UDrawer>
</template>
