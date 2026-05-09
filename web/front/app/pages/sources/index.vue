<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type { DbSource } from "~/api/generated/data-contracts";
import ChangeSyncStatus from "~/sync/change-sync-status/components/change-sync-status.vue";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { formatDate } from "~~/utils/format-date";
import { getErrorMessage } from "~~/utils/get-error-message";
import { isAbortError } from "~~/utils/is-abort-error";

const toast = useToast();

let lastFetchController: AbortController | null = null;

const data = ref<DbSource[]>([]);

const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

useHead({
    title: "Sources",
});

const pagination = ref({
    pageIndex: 0,
    pageSize: 12,
    total: 0,
});

const fetchData = async () => {
    try {
        if (lastFetchController) lastFetchController.abort();
        lastFetchController = new AbortController();
        const response = await api.getAllSyncRecords(lastFetchController.signal);

        data.value = response.list ?? [];
        pagination.value = {
            ...pagination.value,
            total: response.total ?? 0,
        };
    } catch (error) {
        if (isAbortError(error)) return;
        toast.add({
            title: "Error",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        console.error("Error fetching data:", error);
    }
};

const fetchWithLoading = createLoadingRequest(fetchData);

const changePage = async (page: number) => {
    pagination.value.pageIndex = page - 1;
    await fetchWithLoading();
};

onMounted(fetchWithLoading);

const updateActiveStatus = (item: DbSource) => {
    const index = data.value.findIndex((record) => record.id === item.id);
    if (index !== -1) {
        data.value.splice(index, 1, item);
    }
};

const columns: TableColumn<DbSource>[] = [
    {
        accessorKey: "id",
        header: "ID",
        meta: { class: { td: "tabular-nums text-muted" } },
    },
    {
        accessorKey: "created_at",
        header: "Created",
        cell: ({ row }) => formatDate(row.getValue("created_at")),
    },
    {
        accessorKey: "name",
        header: "Name",
    },
    {
        accessorKey: "url",
        header: "URL",
        cell: ({ row }) => {
            const url = row.original.name ?? "";
            return h(
                "span",
                { class: "block max-w-[36ch] truncate font-mono text-xs", title: url },
                url,
            );
        },
    },
    {
        accessorKey: "active",
        header: () => h("div", { class: "text-right" }, "Active"),
        cell: ({ row }) =>
            h(
                "div",
                { class: "flex justify-end" },
                h(ChangeSyncStatus, {
                    record: row.original,
                    onUpdate: updateActiveStatus,
                }),
            ),
    },
];
</script>

<template>
    <div class="h-[calc(100vh-var(--ui-header-height))] flex flex-col">
        <div class="flex-1 min-h-0 overflow-auto pt-4">
            <UContainer>
                <UTable
                    v-model:pagination="pagination"
                    :loading="isLoading"
                    sticky="header"
                    empty="No data"
                    :data="data"
                    :columns="columns"
                    :ui="{ root: 'relative' }"
                />
            </UContainer>
        </div>

        <UContainer class="shrink-0 pb-4">
            <div class="flex justify-center border-t border-default pt-4">
                <UPagination
                    v-if="pagination.total > pagination.pageSize"
                    :default-page="pagination.pageIndex + 1"
                    :items-per-page="pagination.pageSize"
                    :total="pagination.total"
                    @update:page="changePage"
                />
            </div>
        </UContainer>
    </div>
</template>
