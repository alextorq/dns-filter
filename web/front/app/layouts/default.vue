<script setup lang="ts">
import { useAuth } from "~~/composables/use-auth";

const items = ref([
    {
        label: "Domains",
        icon: "i-lucide-book-open",
        to: "/domains",
    },
    {
        label: "Sources",
        icon: "i-lucide-refresh-cw",
        to: "/sources",
    },
    {
        label: "Statistic",
        icon: "i-lucide-pie-chart",
        to: "/statistic",
    },
    {
        label: "Suggest",
        icon: "i-lucide-lightbulb",
        to: "/suggest",
    },
    {
        label: "Inspect",
        icon: "i-lucide-radar",
        to: "/inspect",
    },
    {
        label: "Clients",
        icon: "i-lucide-user",
        to: "/clients",
    },
    {
        label: "Traffic",
        icon: "i-lucide-activity",
        to: "/traffic",
    },
    {
        label: "Settings",
        icon: "i-lucide-settings",
        to: "/settings",
    },
]);

const { user, logout } = useAuth();

const onLogout = async () => {
    await logout();
    await navigateTo("/auth");
};
</script>

<template>
    <UApp>
        <UHeader to="/">
            <template #title>
                <AppLogo />
            </template>
            <UNavigationMenu :items="items" />
            <template #right>
                <UColorModeButton />
                <UButton
                    v-if="user"
                    icon="i-lucide-log-out"
                    variant="ghost"
                    color="neutral"
                    :label="user.login"
                    @click="onLogout"
                />
            </template>
            <template #body>
                <UNavigationMenu :items="items" />
            </template>
        </UHeader>
        <UMain>
            <NuxtPage />
        </UMain>
    </UApp>
</template>

<style>
@import "tailwindcss";
@import "@nuxt/ui";
</style>
