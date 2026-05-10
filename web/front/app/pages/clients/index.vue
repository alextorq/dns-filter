<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type { DbExcludeClient } from "~/api/generated/data-contracts";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { formatDate } from "~~/utils/format-date";
import { getErrorMessage } from "~~/utils/get-error-message";
import { isAbortError } from "~~/utils/is-abort-error";
import AddClientModal from "./components/add-client-modal.vue";
import ChangeClientStatus from "./components/change-client-status.vue";
import DeleteClient from "./components/delete-client.vue";

const toast = useToast();

let lastFetchController: AbortController | null = null;

const data = ref<DbExcludeClient[]>([]);
const globalFilter = ref("");

const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

useHead({
    title: "Exclude Clients",
});

const pagination = ref({
    pageIndex: 0,
    pageSize: 12,
    total: 0,
});

const filtered = computed(() => {
    const q = globalFilter.value.trim().toLowerCase();
    if (!q) return data.value;
    return data.value.filter((c) => (c.user_id ?? "").toLowerCase().includes(q));
});

const paginated = computed(() => {
    const start = pagination.value.pageIndex * pagination.value.pageSize;
    return filtered.value.slice(start, start + pagination.value.pageSize);
});

const filteredTotal = computed(() => filtered.value.length);

watch(globalFilter, () => {
    pagination.value.pageIndex = 0;
});

const fetchData = async () => {
    try {
        if (lastFetchController) lastFetchController.abort();
        lastFetchController = new AbortController();
        const response = await api.getAllExcludeClients(lastFetchController.signal);

        data.value = response.list ?? [];
        pagination.value = {
            ...pagination.value,
            total: response.total ?? data.value.length,
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

const changePage = (page: number) => {
    pagination.value.pageIndex = page - 1;
};

onMounted(fetchWithLoading);

const updateActiveStatus = (item: DbExcludeClient) => {
    const index = data.value.findIndex((record) => record.id === item.id);
    if (index !== -1) {
        data.value.splice(index, 1, item);
    }
};

const deleteClient = (item: DbExcludeClient) => {
    const index = data.value.findIndex((record) => record.id === item.id);
    if (index !== -1) {
        data.value.splice(index, 1);
    }
    toast.add({
        title: "Deleted",
        description: `${item.user_id} removed from exclusions.`,
        duration: 3000,
    });
};

const columns: TableColumn<DbExcludeClient>[] = [
    {
        accessorKey: "id",
        header: "ID",
        meta: { class: { td: "tabular-nums text-muted" } },
    },
    {
        accessorKey: "user_id",
        header: "Client IP",
        cell: ({ row }) => {
            const id = row.original.user_id ?? "";
            return h("span", { class: "font-mono", title: id }, id);
        },
    },
    {
        accessorKey: "created_at",
        header: "Created",
        cell: ({ row }) => formatDate(row.getValue("created_at")),
    },
    {
        accessorKey: "updated_at",
        header: "Updated",
        cell: ({ row }) => formatDate(row.getValue("updated_at")),
    },
    {
        accessorKey: "active",
        header: () => h("div", { class: "text-right" }, "Active"),
        cell: ({ row }) =>
            h(
                "div",
                { class: "flex justify-end" },
                h(ChangeClientStatus, {
                    record: row.original,
                    onUpdate: updateActiveStatus,
                }),
            ),
    },
    {
        id: "actions",
        header: "",
        cell: ({ row }) =>
            h(
                "div",
                { class: "flex justify-end" },
                h(DeleteClient, {
                    record: row.original,
                    onDelete: deleteClient,
                }),
            ),
    },
];
</script>

<template>
    <div class="h-[calc(100vh-var(--ui-header-height))] flex flex-col">
        <UContainer class="shrink-0 pt-4">
            <div
                class="flex flex-wrap gap-3 px-4 py-3.5 justify-between items-center border-b border-accented"
            >
                <UInput
                    v-model="globalFilter"
                    class="max-w-sm"
                    icon="i-lucide-search"
                    placeholder="Search by IP"
                />
                <AddClientModal @success="fetchWithLoading" />
            </div>
        </UContainer>

        <div class="flex-1 min-h-0 overflow-auto">
            <UContainer>
                <UTable
                    :loading="isLoading"
                    sticky="header"
                    empty="No clients"
                    :data="paginated"
                    :columns="columns"
                    :ui="{ root: 'relative' }"
                />
            </UContainer>
        </div>

        <UContainer class="shrink-0 pb-4">
            <div class="flex justify-center border-t border-default pt-4">
                <UPagination
                    v-if="filteredTotal > pagination.pageSize"
                    :default-page="pagination.pageIndex + 1"
                    :items-per-page="pagination.pageSize"
                    :total="filteredTotal"
                    @update:page="changePage"
                />
            </div>
        </UContainer>
    </div>
</template>
