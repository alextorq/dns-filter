<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type { DbClient } from "~/api/generated/data-contracts";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { formatDate } from "~~/utils/format-date";
import { getErrorMessage } from "~~/utils/get-error-message";
import { isAbortError } from "~~/utils/is-abort-error";
import AddClientModal from "./components/add-client-modal.vue";
import DeleteClient from "./components/delete-client.vue";
import FilterToggle from "./components/filter-toggle.vue";

const toast = useToast();

let lastFetchController: AbortController | null = null;

const data = ref<DbClient[]>([]);
const globalFilter = ref("");

const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

useHead({
    title: "Clients",
});

const pagination = ref({
    pageIndex: 0,
    pageSize: 12,
    total: 0,
});

const filtered = computed(() => {
    const q = globalFilter.value.trim().toLowerCase();
    if (!q) return data.value;
    return data.value.filter((c) => {
        const haystack = [c.ip, c.mac, c.name, c.hostname, c.vendor]
            .filter(Boolean)
            .join(" ")
            .toLowerCase();
        return haystack.includes(q);
    });
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
        const response = await api.getAllClients(lastFetchController.signal);

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

const updateClient = (item: DbClient) => {
    const index = data.value.findIndex((record) => record.id === item.id);
    if (index !== -1) {
        data.value.splice(index, 1, item);
    }
};

const removeClient = (item: DbClient) => {
    const index = data.value.findIndex((record) => record.id === item.id);
    if (index !== -1) {
        data.value.splice(index, 1);
    }
    toast.add({
        title: "Deleted",
        description: `${item.name || item.ip || `#${item.id}`} removed.`,
        duration: 3000,
    });
};

const columns: TableColumn<DbClient>[] = [
    {
        accessorKey: "id",
        header: "ID",
        meta: { class: { td: "tabular-nums text-muted" } },
    },
    {
        accessorKey: "name",
        header: "Name",
        cell: ({ row }) => row.original.name || h("span", { class: "text-muted" }, "—"),
    },
    {
        accessorKey: "ip",
        header: "IP",
        cell: ({ row }) => {
            const ip = row.original.ip ?? "";
            return ip
                ? h("span", { class: "font-mono", title: ip }, ip)
                : h("span", { class: "text-muted" }, "—");
        },
    },
    {
        accessorKey: "mac",
        header: "MAC",
        cell: ({ row }) => {
            const mac = row.original.mac ?? "";
            return mac
                ? h("span", { class: "font-mono text-muted text-sm" }, mac)
                : h("span", { class: "text-muted" }, "—");
        },
    },
    {
        accessorKey: "vendor",
        header: "Vendor",
        cell: ({ row }) => row.original.vendor || h("span", { class: "text-muted" }, "—"),
    },
    {
        accessorKey: "updated_at",
        header: "Updated",
        cell: ({ row }) => formatDate(row.getValue("updated_at")),
    },
    {
        accessorKey: "filtered",
        header: () => h("div", { class: "text-right" }, "Filter"),
        cell: ({ row }) =>
            h(
                "div",
                { class: "flex justify-end" },
                h(FilterToggle, {
                    record: row.original,
                    onUpdate: updateClient,
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
                    onDelete: removeClient,
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
                    placeholder="Search by IP, MAC, name…"
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
