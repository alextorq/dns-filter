<script setup lang="ts">
import { api } from "~/api";
import { useToggle } from "~~/composables/use-toggle";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

const emit = defineEmits(["success"]);

const toast = useToast();
const { isActive, closeHandler, openHandler } = useToggle();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

const touched = ref(false);
const validationError = ref("");

const state = reactive({
    user_id: "",
});

const validation = (): string | null => {
    const value = state.user_id.trim();
    if (!value) {
        return "Client IP is required.";
    }
    const ipv4 = /^(25[0-5]|2[0-4]\d|[01]?\d?\d)(\.(25[0-5]|2[0-4]\d|[01]?\d?\d)){3}$/;
    if (!ipv4.test(value)) {
        return "Enter a valid IPv4 address (e.g. 192.168.1.10).";
    }
    return null;
};

const runValidation = () => {
    validationError.value = validation() ?? "";
    return !validationError.value;
};

watch(
    () => state.user_id,
    () => {
        if (touched.value) runValidation();
    },
);

const reset = () => {
    state.user_id = "";
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
    await api.addExcludeClient({ user_id: state.user_id.trim() });
    toast.add({
        title: "Success",
        description: "New client was added.",
        duration: 3000,
    });
    emit("success");
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
        <UButton size="xl" label="Add Client" icon="i-lucide-plus" @click="openHandler" />

        <template #header>
            <h1 class="text-lg font-semibold">Add Client</h1>
        </template>

        <template #body>
            <UForm
                id="add-client-form"
                :state="state"
                class="w-full max-w-xl"
                @submit="submitWithLoading"
            >
                <UFormField
                    label="Client IP"
                    name="user_id"
                    required
                    :error="validationError"
                    help="IPv4 address of the device that should bypass filtering."
                >
                    <UInput
                        v-model="state.user_id"
                        size="xl"
                        class="w-full"
                        placeholder="192.168.1.10"
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
                    label="Add Client"
                    type="submit"
                    form="add-client-form"
                    :loading="isLoading"
                />
            </div>
        </template>
    </UDrawer>
</template>
