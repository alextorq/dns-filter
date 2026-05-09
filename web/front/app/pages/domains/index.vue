<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type { DbBlockList } from "~/api/generated/data-contracts";
import AddDomainModal from "~/domain/add-new-domain/components/add-domain-modal.vue";
import ChangeStatus from "~/domain/change-domain-status/components/change-status.vue";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { useDebounceFn } from "~~/composables/use-debounce-fn";
import { formatDate } from "~~/utils/format-date";
import { getErrorMessage } from "~~/utils/get-error-message";
import { isAbortError } from "~~/utils/is-abort-error";

let lastFetchController: AbortController | null = null;

const toast = useToast();

const data = ref<DbBlockList[]>([]);
const globalFilter = ref("");
const source = ref<string | null>(null);

const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

const pagination = ref({
    pageIndex: 0,
    pageSize: 12,
    total: 0,
});

useHead({
    title: "Domains",
});

const fetchData = async () => {
    try {
        if (lastFetchController) lastFetchController.abort();
        lastFetchController = new AbortController();
        const response = await api.getAllDnsRecords(
            {
                limit: pagination.value.pageSize,
                offset: pagination.value.pageIndex * pagination.value.pageSize || 0,
                filter: globalFilter.value,
                source: source.value || "",
            },
            lastFetchController.signal,
        );

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

const changeFilter = async () => {
    pagination.value.pageIndex = 0;
    await fetchWithLoading();
};

const { debounced: debouncedFilter } = useDebounceFn(changeFilter, 300);

watch(globalFilter, () => debouncedFilter());
watch(source, () => changeFilter());

const changePage = async (page: number) => {
    pagination.value.pageIndex = page - 1;
    await fetchWithLoading();
};

onMounted(fetchWithLoading);

const updateActiveStatus = (item: DbBlockList) => {
    const index = data.value.findIndex((record) => record.id === item.id);
    if (index !== -1) {
        data.value.splice(index, 1, item);
    }
};

const columns: TableColumn<DbBlockList>[] = [
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
        accessorKey: "url",
        header: "Domain",
        cell: ({ row }) => {
            const url = row.original.url ?? "";
            return h(
                "span",
                { class: "block max-w-[28ch] truncate font-mono", title: url },
                url,
            );
        },
    },
    {
        accessorKey: "source",
        header: "Source",
    },
    {
        accessorKey: "active",
        header: () => h("div", { class: "text-right" }, "Active"),
        cell: ({ row }) =>
            h(
                "div",
                { class: "flex justify-end" },
                h(ChangeStatus, {
                    record: row.original,
                    onUpdate: updateActiveStatus,
                }),
            ),
    },
];
</script>

<template>
    <div class="h-[calc(100vh-var(--ui-header-height))] flex flex-col">
        <UContainer class="shrink-0 pt-4">
            <div class="flex flex-wrap gap-3 px-4 py-3.5 justify-between border-b border-accented">
                <div class="flex flex-wrap items-center gap-3">
                    <UInput
                        v-model="globalFilter"
                        class="max-w-sm"
                        icon="i-lucide-search"
                        placeholder="Search domain"
                    />

                    <USelect
                        v-model="source"
                        class="w-40"
                        placeholder="Source"
                        :items="[
                            { label: 'All', value: null },
                            { label: 'StevenBlack', value: 'StevenBlack' },
                            { label: 'User', value: 'User' },
                            { label: 'EasyList', value: 'EasyList' },
                            { label: 'SuggestedToBlock', value: 'SuggestedToBlock' },
                        ]"
                    />
                </div>
                <AddDomainModal />
            </div>
        </UContainer>

        <div class="flex-1 min-h-0 overflow-auto">
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
