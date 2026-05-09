<script setup lang="ts">
import { API_HOST } from "~/api";
import { downloadFile } from "~~/utils/download";

const downloadDb = () => {
    downloadFile(API_HOST + "/config/db/download", "filter.sqlite");
};
</script>

<template>
    <UButton
        size="lg"
        icon="i-lucide-download"
        variant="soft"
        label="Download SQLite snapshot"
        @click="downloadDb"
    />
</template>
