<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type { DbSuggestBlock } from "~/api/generated/data-contracts";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { useDebounceFn } from "~~/composables/use-debounce-fn";
import { UButton } from "#components";
import { getErrorMessage } from "~~/utils/get-error-message";
import { isAbortError } from "~~/utils/is-abort-error";

useHead({
    title: "Suggest",
});

const toast = useToast();

let lastFetchController: AbortController | null = null;

const records = ref<DbSuggestBlock[]>([]);
const globalFilter = ref("");

const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

const pagination = ref({
    pageIndex: 0,
    pageSize: 12,
    total: 0,
});

const fetchData = async () => {
    try {
        if (lastFetchController) lastFetchController.abort();
        lastFetchController = new AbortController();
        const response = await api.getAllSuggestRecords(
            {
                limit: pagination.value.pageSize,
                offset: pagination.value.pageIndex * pagination.value.pageSize || 0,
                filter: globalFilter.value,
                active: true,
            },
            lastFetchController.signal,
        );

        records.value = response.list ?? [];
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

const changePage = async (page: number) => {
    pagination.value.pageIndex = page - 1;
    await fetchWithLoading();
};

onMounted(fetchWithLoading);

const blockDomain = async (item: DbSuggestBlock) => {
    try {
        await api.addSuggestToBlock(item);
        await fetchWithLoading();
        toast.add({
            title: "Blocked",
            description: `${item.domain} added to the blocklist.`,
            duration: 3000,
        });
    } catch (e) {
        toast.add({
            title: "Error",
            description: getErrorMessage(e),
            duration: 5000,
            color: "error",
        });
        console.error("Error creating domain:", e);
    }
};

const columns: TableColumn<DbSuggestBlock>[] = [
    {
        accessorKey: "id",
        header: "ID",
        meta: { class: { td: "tabular-nums text-muted" } },
    },
    {
        accessorKey: "domain",
        header: "Domain",
        cell: ({ row }) => {
            const domain = row.original.domain ?? "";
            return h(
                "span",
                { class: "block max-w-[28ch] truncate font-mono", title: domain },
                domain,
            );
        },
    },
    {
        accessorKey: "score",
        header: "Score",
        meta: { class: { td: "tabular-nums" } },
    },
    {
        accessorKey: "reasons",
        header: "Reason",
        cell: ({ row }) =>
            h(
                "ul",
                { class: "whitespace-normal break-words text-xs space-y-0.5" },
                (row.original.reasons ?? "")
                    .split("\n")
                    .filter(Boolean)
                    .map((reason: string) => h("li", reason)),
            ),
    },
    {
        id: "actions",
        header: () => h("div", { class: "text-right" }, "Actions"),
        cell: ({ row }) =>
            h(
                "div",
                { class: "flex justify-end" },
                h(
                    UButton,
                    {
                        size: "sm",
                        color: "primary",
                        icon: "i-lucide-shield-x",
                        label: "Block",
                        onClick: () => blockDomain(row.original),
                    },
                ),
            ),
    },
];
</script>

<template>
    <div class="h-[calc(100vh-var(--ui-header-height))] flex flex-col">
        <UContainer class="shrink-0 pt-4">
            <div class="flex px-4 py-3.5 justify-between border-b border-accented">
                <UInput
                    v-model="globalFilter"
                    class="max-w-sm"
                    icon="i-lucide-search"
                    placeholder="Search domain"
                />
            </div>
        </UContainer>

        <div class="flex-1 min-h-0 overflow-auto">
            <UContainer>
                <UTable
                    v-model:pagination="pagination"
                    :loading="isLoading"
                    sticky="header"
                    empty="No suggested domains"
                    :data="records"
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
