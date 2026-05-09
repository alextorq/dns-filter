<script setup lang="ts">
import * as z from "zod";
import type { FormSubmitEvent } from "@nuxt/ui";
import { getErrorMessage } from "~~/utils/get-error-message";
import { useAuth } from "~~/composables/use-auth";

definePageMeta({ layout: "auth" });

useHead({
    title: "Auth",
});

const fields = [
    {
        name: "login",
        type: "text" as const,
        label: "Login",
        placeholder: "Enter your login",
        required: true,
    },
    {
        name: "password",
        label: "Password",
        type: "password" as const,
        placeholder: "Enter your password",
        required: true,
    },
];

const schema = z.object({
    login: z.string().min(1, "Required"),
    password: z.string().min(1, "Required"),
});

type Schema = z.output<typeof schema>;

const toast = useToast();
const { login } = useAuth();

async function onSubmit(payload: FormSubmitEvent<Schema>) {
    try {
        await login(payload.data.login, payload.data.password);
        await navigateTo("/");
    } catch (error) {
        toast.add({
            title: "Login failed",
            description: getErrorMessage(error),
            duration: 4000,
            color: "error",
        });
    }
}
</script>

<template>
    <div class="flex flex-col items-center justify-center gap-4 p-4">
        <UPageCard class="w-full max-w-md">
            <UAuthForm
                :schema="schema"
                title="Login"
                description="Enter your credentials to access your account."
                icon="i-lucide-user"
                :fields="fields"
                @submit="onSubmit"
            />
        </UPageCard>
    </div>
</template>
