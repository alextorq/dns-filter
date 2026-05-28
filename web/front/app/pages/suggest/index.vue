<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type {
    CollectSignalDescriptor,
    DbSuggestBlock,
    DbSuggestBlockReason,
} from "~/api/generated/data-contracts";
import InspectDrawer from "~/domain/inspect/components/inspect-drawer.vue";
import InspectTags from "~/domain/inspect/components/inspect-tags.vue";
import { useInspectRegistry } from "~~/composables/use-inspect-registry";
import { usePaginatedList } from "~~/composables/use-paginated-list";
import { UButton } from "#components";
import { getErrorMessage } from "~~/utils/get-error-message";
import { isAbortError } from "~~/utils/is-abort-error";

useHead({ title: "Suggest" });

const toast = useToast();

// Catalog of reason codes (label + description) is owned by the backend.
// Frontend just renders what comes from /api/suggest-to-block/codes.
const signalCatalog = ref<CollectSignalDescriptor[]>([]);
const labelByCode = computed(() => {
    const map: Record<string, string> = {};
    for (const s of signalCatalog.value) {
        if (s.code) map[s.code] = s.label ?? s.code;
    }
    return map;
});
const labelForReason = (r: DbSuggestBlockReason): string => {
    const base = labelByCode.value[r.code ?? ""] ?? r.code ?? "";
    return r.match ? `${base}: ${r.match}` : base;
};

// Multi-select filter — stores the chosen reason codes.
const selectedCodes = ref<string[]>([]);
const codeItems = computed(() =>
    signalCatalog.value
        .filter((s) => Boolean(s.code))
        .map((s) => ({ label: s.label ?? s.code!, value: s.code! })),
);

const {
    data: records,
    filter: globalFilter,
    pagination,
    isLoading,
    refresh,
    resetAndFetch,
    changePage,
} = usePaginatedList<DbSuggestBlock>(({ limit, offset, filter, signal }) =>
    api.getAllSuggestRecords(
        {
            limit,
            offset,
            filter,
            active: true,
            codes: selectedCodes.value.length ? selectedCodes.value : undefined,
        },
        signal,
    ),
);

watch(selectedCodes, () => resetAndFetch());

// Kick off a background inspect scan for every domain that appears in the
// table so the user can see verdict tags in the row without opening the
// drawer. The registry caches results across pages and dedups duplicates.
const { register: registerInspect } = useInspectRegistry();
watch(
    records,
    (rows) => {
        for (const r of rows ?? []) {
            if (r.domain) registerInspect(r.domain);
        }
    },
    { immediate: true },
);

let catalogFetchController: AbortController | null = null;

const fetchSignalCatalog = async () => {
    if (catalogFetchController) catalogFetchController.abort();
    catalogFetchController = new AbortController();
    try {
        const response = await api.getSuggestSignalCodes(catalogFetchController.signal);
        signalCatalog.value = response.list ?? [];
    } catch (error) {
        if (isAbortError(error)) return;
        console.error("Error fetching signal catalog:", error);
    }
};

onScopeDispose(() => catalogFetchController?.abort());

onMounted(() => {
    refresh();
    fetchSignalCatalog();
});

const blockDomain = async (item: DbSuggestBlock) => {
    try {
        await api.addSuggestToBlock(item);
        await refresh();
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
        meta: {
            class: {
                th: "hidden lg:table-cell",
                td: "hidden lg:table-cell tabular-nums text-muted",
            },
        },
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
        meta: { class: { th: "hidden sm:table-cell", td: "hidden sm:table-cell tabular-nums" } },
    },
    {
        accessorKey: "reasons",
        header: "Reason",
        meta: { class: { th: "hidden lg:table-cell", td: "hidden lg:table-cell" } },
        cell: ({ row }) =>
            h(
                "ul",
                { class: "whitespace-normal break-words text-xs space-y-0.5" },
                (row.original.reasons ?? []).map((reason) => h("li", labelForReason(reason))),
            ),
    },
    {
        id: "scan",
        header: "Scan",
        meta: { class: { th: "hidden md:table-cell", td: "hidden md:table-cell" } },
        cell: ({ row }) => h(InspectTags, { domain: row.original.domain ?? "" }),
    },
    {
        id: "actions",
        header: () => h("div", { class: "text-right" }, "Actions"),
        cell: ({ row }) =>
            h("div", { class: "flex justify-end gap-1" }, [
                h(InspectDrawer, {
                    domain: row.original.domain ?? "",
                    onBlock: () => blockDomain(row.original),
                }),
                h(UButton, {
                    size: "sm",
                    color: "primary",
                    icon: "i-lucide-shield-x",
                    label: "Block",
                    onClick: () => blockDomain(row.original),
                }),
            ]),
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

                    <USelectMenu
                        v-model="selectedCodes"
                        multiple
                        class="min-w-60"
                        placeholder="Filter by reasons"
                        :items="codeItems"
                        value-key="value"
                    />
                </div>
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
