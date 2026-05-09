<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type { DbSource } from "~/api/generated/data-contracts";
import ChangeSyncStatus from "~/sync/change-sync-status/components/change-sync-status.vue";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

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
        const message = getErrorMessage(error);
        toast.add({
            title: "Error",
            description: message,
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
    try {
        const index = data.value.findIndex((record) => record.id === item.id);
        if (index !== -1) {
            data.value.splice(index, 1, item);
        }
    } catch (error) {
        console.error("Error updating status:", error);
    }
};

const columns: TableColumn<DbSource>[] = [
    {
        accessorKey: "id",
        header: "id",
    },
    {
        accessorKey: "created_at",
        header: "Date of creation",
        cell: ({ row }) => {
            return new Date(row.getValue("created_at")).toLocaleString("en-En", {
                day: "numeric",
                month: "short",
                hour: "2-digit",
                minute: "2-digit",
                hour12: false,
            });
        },
    },
    {
        accessorKey: "name",
        header: "Name",
    },
    {
        accessorKey: "url",
        header: "URL",
    },
    {
        accessorKey: "active",
        header: () => h("div", { class: "text-right" }, "Active"),
        cell: ({ row }) => {
            return h(ChangeSyncStatus, {
                record: row.original,
                onUpdate: updateActiveStatus,
            });
        },
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
                    :default-page="pagination.pageIndex + 1"
                    :items-per-page="pagination.pageSize"
                    :total="pagination.total"
                    @update:page="changePage"
                />
            </div>
        </UContainer>
    </div>
</template>
