<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type {
    CollectSignalDescriptor,
    DbBlockList,
    DbBlockListReason,
    DbBlockListSource,
    DbSource,
} from "~/api/generated/data-contracts";
import AddDomainModal from "~/domain/add-new-domain/components/add-domain-modal.vue";
import ChangeStatus from "~/domain/change-domain-status/components/change-status.vue";
import { usePaginatedList } from "~~/composables/use-paginated-list";
import { formatDate } from "~~/utils/format-date";
import { isAbortError } from "~~/utils/is-abort-error";

const sources = ref<DbSource[]>([]);
const source = ref<string | null>(null);

// Reason codes (subdomain_of_blocked, …) attached to auto-blocked rows reuse
// the same backend catalog as the Suggest page — /api/suggest-to-block/codes
// owns the human-readable labels (#95).
const signalCatalog = ref<CollectSignalDescriptor[]>([]);
const labelByCode = computed(() => {
    const map: Record<string, string> = {};
    for (const s of signalCatalog.value) {
        if (s.code) map[s.code] = s.label ?? s.code;
    }
    return map;
});
const labelForReason = (r: DbBlockListReason): string => {
    const base = labelByCode.value[r.code ?? ""] ?? r.code ?? "";
    return r.match ? `${base}: ${r.match}` : base;
};

const sourceItems = computed(() => [
    { label: "All", value: null as string | null },
    ...sources.value
        .map((s) => s.name)
        .filter((name): name is DbBlockListSource => Boolean(name))
        .map((name) => ({ label: name, value: name })),
]);

useHead({
    title: "Domains",
});

const {
    data,
    filter: globalFilter,
    pagination,
    isLoading,
    refresh,
    resetAndFetch,
    changePage,
} = usePaginatedList<DbBlockList>(({ limit, offset, filter, signal }) =>
    api.getAllDnsRecords({ limit, offset, filter, source: source.value || "" }, signal),
);

watch(source, () => resetAndFetch());

const fetchSources = async () => {
    try {
        const response = await api.getAllSyncRecords(new AbortController().signal);
        sources.value = response.list ?? [];
    } catch (error) {
        if (isAbortError(error)) return;
        console.error("Error fetching sources:", error);
    }
};

const fetchSignalCatalog = async () => {
    try {
        const response = await api.getSuggestSignalCodes(new AbortController().signal);
        signalCatalog.value = response.list ?? [];
    } catch (error) {
        if (isAbortError(error)) return;
        console.error("Error fetching signal catalog:", error);
    }
};

onMounted(() => {
    refresh();
    fetchSources();
    fetchSignalCatalog();
});

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
            return h("span", { class: "block max-w-[28ch] truncate font-mono", title: url }, url);
        },
    },
    {
        accessorKey: "source",
        header: "Source",
    },
    {
        accessorKey: "reasons",
        header: "Reason",
        cell: ({ row }) => {
            const reasons = row.original.reasons ?? [];
            if (reasons.length === 0) {
                return h("span", { class: "text-muted" }, "—");
            }
            return h(
                "ul",
                { class: "whitespace-normal break-words text-xs space-y-0.5" },
                reasons.map((reason) => h("li", labelForReason(reason))),
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
                        :items="sourceItems"
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
