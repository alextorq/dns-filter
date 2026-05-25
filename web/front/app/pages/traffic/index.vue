<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type { DiscoveryDevice, WebDeviceDTO, WebDomainCountDTO } from "~/api/generated/data-contracts";
import { useTrafficDashboard, type VerdictFilter } from "~~/composables/use-traffic-dashboard";
import { getErrorMessage } from "~~/utils/get-error-message";
import { isAbortError } from "~~/utils/is-abort-error";
import { formatDate } from "~~/utils/format-date";

useHead({ title: "Traffic" });

const t = useTrafficDashboard();

const activeTab = ref("devices");
const tabs = [
    { label: "Devices", icon: "i-lucide-monitor-smartphone", value: "devices" },
    { label: "Top domains", icon: "i-lucide-trophy", value: "top" },
];

const verdictItems: { label: string; value: VerdictFilter }[] = [
    { label: "All", value: "all" },
    { label: "Blocked", value: "blocked" },
    { label: "Allowed", value: "allowed" },
];

const formatNumber = (n: number | undefined) => (n ?? 0).toLocaleString("en-US");

// --- Device list ---
const deviceColumns: TableColumn<WebDeviceDTO>[] = [
    {
        accessorKey: "client_value",
        header: "Device",
        cell: ({ row }) => {
            const d = row.original;
            const isMac = d.client_kind === "mac";
            const vendor = d.vendor || "";
            const ip = d.current_ip || "";
            const title = isMac && vendor ? vendor : ip || (d.client_value ?? "");
            const subtitleParts = [d.client_value ?? "—"];
            if (isMac && ip) subtitleParts.push(ip);
            return h("div", { class: "flex flex-col gap-0.5" }, [
                h(
                    "div",
                    { class: "flex items-center gap-2" },
                    [
                        h(
                            resolveComponent("UBadge"),
                            {
                                size: "sm",
                                variant: "subtle",
                                color: isMac ? "primary" : "neutral",
                            },
                            () => (isMac ? "MAC" : "IP"),
                        ),
                        h("span", { class: "font-medium truncate" }, title || "—"),
                    ],
                ),
                h(
                    "span",
                    { class: "font-mono text-xs text-muted truncate" },
                    subtitleParts.join("  ·  "),
                ),
            ]);
        },
    },
    {
        accessorKey: "allowed_count",
        header: () => h("div", { class: "text-right" }, "Allowed"),
        cell: ({ row }) =>
            h(
                "div",
                { class: "text-right tabular-nums text-success" },
                formatNumber(row.original.allowed_count),
            ),
    },
    {
        accessorKey: "blocked_count",
        header: () => h("div", { class: "text-right" }, "Blocked"),
        cell: ({ row }) =>
            h(
                "div",
                { class: "text-right tabular-nums text-error" },
                formatNumber(row.original.blocked_count),
            ),
    },
    {
        accessorKey: "last_seen",
        header: "Last seen",
        cell: ({ row }) =>
            row.original.last_seen
                ? formatDate(row.original.last_seen)
                : h("span", { class: "text-muted" }, "—"),
    },
    {
        id: "actions",
        header: "",
        cell: ({ row }) =>
            h(
                "div",
                { class: "flex justify-end" },
                h(
                    resolveComponent("UButton"),
                    {
                        size: "sm",
                        variant: "ghost",
                        icon: "i-lucide-chevron-right",
                        color:
                            t.selectedKey.value === t.deviceKey(row.original) ? "primary" : "neutral",
                        onClick: () => onSelectDevice(row.original),
                    },
                    () => "Domains",
                ),
            ),
    },
];

const domainColumns: TableColumn<WebDomainCountDTO>[] = [
    {
        accessorKey: "domain",
        header: "Domain",
        cell: ({ row }) =>
            h("span", { class: "font-mono text-sm truncate", title: row.original.domain ?? "" }, row.original.domain ?? "—"),
    },
    {
        accessorKey: "count",
        header: () => h("div", { class: "text-right" }, "Queries"),
        cell: ({ row }) =>
            h("div", { class: "text-right tabular-nums" }, formatNumber(row.original.count)),
    },
];

const topColumns: TableColumn<WebDomainCountDTO>[] = [
    {
        id: "rank",
        header: "#",
        cell: ({ row }) =>
            h("span", { class: "tabular-nums text-muted" }, String(row.index + 1)),
    },
    {
        accessorKey: "domain",
        header: "Domain",
        cell: ({ row }) =>
            h("span", { class: "font-mono text-sm truncate", title: row.original.domain ?? "" }, row.original.domain ?? "—"),
    },
    {
        accessorKey: "count",
        header: () => h("div", { class: "text-right" }, "Queries"),
        cell: ({ row }) =>
            h("div", { class: "text-right tabular-nums" }, formatNumber(row.original.count)),
    },
];

const onSelectDevice = (device: WebDeviceDTO) => {
    void t.selectDevice(device);
};

const onDomainsPageChange = (page: number) => {
    void t.changeDomainsPage(page);
};

const selectedDeviceTitle = computed(() => {
    const d = t.selectedDevice.value;
    if (!d) return "";
    if (d.client_kind === "mac") return d.vendor || d.current_ip || d.client_value || "Device";
    return d.current_ip || d.client_value || "Device";
});

// Re-fetch the drill-down when the verdict filter changes (resets to page 1).
watch(t.blockedFilter, () => {
    if (t.selectedDevice.value) void t.reloadDomainsFromStart();
});

// Re-fetch devices + (if open) the drill-down when the day range changes.
watch([t.from, t.to], () => {
    void t.loadDevices();
    if (t.selectedDevice.value) void t.reloadDomainsFromStart();
});

// Re-fetch top domains when its verdict filter changes.
watch(t.topBlockedFilter, () => {
    if (activeTab.value === "top") void t.loadTopDomains();
});

// Lazy-load the top-domains tab the first time it is opened.
const topLoadedOnce = ref(false);
watch(activeTab, (tab) => {
    if (tab === "top" && !topLoadedOnce.value) {
        topLoadedOnce.value = true;
        void t.loadTopDomains();
    }
});

// --- Scan LAN (reuses the existing clients/discover endpoint, best-effort) ---
const scanLoading = ref(false);
const scanError = ref<string | null>(null);
const scanWarnings = ref<string[]>([]);
const scanResults = ref<DiscoveryDevice[]>([]);
const scanDone = ref(false);
let scanController: AbortController | null = null;

const scanLan = async () => {
    if (scanController) scanController.abort();
    scanController = new AbortController();
    scanLoading.value = true;
    scanError.value = null;
    try {
        const res = await api.discoverNetwork(scanController.signal);
        scanResults.value = res.devices ?? [];
        scanWarnings.value = res.errors ?? [];
        scanDone.value = true;
    } catch (error) {
        if (isAbortError(error)) return;
        scanError.value = getErrorMessage(error);
        console.error("Scan LAN failed:", error);
    } finally {
        scanLoading.value = false;
    }
};

onMounted(() => {
    void t.loadDevices();
});
</script>

<template>
    <div class="h-[calc(100vh-var(--ui-header-height))] flex flex-col">
        <UContainer class="shrink-0 pt-4">
            <UTabs v-model="activeTab" :items="tabs" :ui="{ list: 'w-full max-w-md' }" />
        </UContainer>

        <!-- ===================== DEVICES TAB ===================== -->
        <template v-if="activeTab === 'devices'">
            <UContainer class="shrink-0 pt-2">
                <div
                    class="flex flex-wrap gap-3 px-4 py-3.5 justify-between items-center border-b border-accented"
                >
                    <div class="flex flex-wrap items-end gap-3">
                        <UFormField label="From" size="xs">
                            <UInput
                                v-model="t.from.value"
                                type="date"
                                placeholder="YYYY-MM-DD"
                            />
                        </UFormField>
                        <UFormField label="To" size="xs">
                            <UInput v-model="t.to.value" type="date" placeholder="YYYY-MM-DD" />
                        </UFormField>
                    </div>
                    <UButton
                        icon="i-lucide-radar"
                        variant="soft"
                        :loading="scanLoading"
                        label="Scan LAN"
                        @click="scanLan"
                    />
                </div>
            </UContainer>

            <div class="flex-1 min-h-0 overflow-auto">
                <UContainer class="flex flex-col gap-4 py-2">
                    <!-- Scan errors / warnings (best-effort, banner) -->
                    <UAlert
                        v-if="scanError"
                        color="error"
                        variant="subtle"
                        icon="i-lucide-circle-x"
                        title="LAN scan failed"
                        :description="scanError"
                        :actions="[
                            {
                                label: 'Retry',
                                color: 'neutral',
                                variant: 'outline',
                                onClick: scanLan,
                            },
                        ]"
                    />
                    <UAlert
                        v-for="(warn, i) in scanWarnings"
                        :key="`scan-warn-${i}`"
                        color="warning"
                        variant="subtle"
                        icon="i-lucide-triangle-alert"
                        title="Partial scan failure"
                        :description="warn"
                    />
                    <UAlert
                        v-if="scanDone && !scanError"
                        color="neutral"
                        variant="subtle"
                        icon="i-lucide-info"
                        :title="`Scan found ${scanResults.length} device${scanResults.length === 1 ? '' : 's'} on the LAN`"
                        description="Use the Clients page to register them. The traffic list below reflects observed DNS queries, not the scan."
                    />

                    <!-- Device list error state -->
                    <UAlert
                        v-if="t.devicesError.value"
                        color="error"
                        variant="subtle"
                        icon="i-lucide-circle-x"
                        title="Failed to load devices"
                        :description="t.devicesError.value"
                        :actions="[
                            {
                                label: 'Retry',
                                color: 'neutral',
                                variant: 'outline',
                                onClick: () => t.loadDevices(),
                            },
                        ]"
                    />

                    <UCard v-if="!t.devicesError.value" :ui="{ body: 'p-0 sm:p-0' }">
                        <UTable
                            :loading="t.devicesLoading.value"
                            sticky="header"
                            empty="No device traffic recorded yet"
                            :data="t.devices.value"
                            :columns="deviceColumns"
                            :ui="{ root: 'relative' }"
                        />
                    </UCard>

                    <!-- Drill-down: selected device domains -->
                    <UCard v-if="t.selectedDevice.value" :ui="{ body: 'p-0 sm:p-0' }">
                        <template #header>
                            <div class="flex flex-wrap items-center justify-between gap-3">
                                <div class="flex flex-col gap-0.5 min-w-0">
                                    <span class="text-xs uppercase tracking-wide text-muted"
                                        >Domains for</span
                                    >
                                    <span class="font-medium truncate">{{
                                        selectedDeviceTitle
                                    }}</span>
                                    <span class="font-mono text-xs text-muted truncate">{{
                                        t.selectedDevice.value.client_value
                                    }}</span>
                                </div>
                                <div class="flex items-center gap-2">
                                    <USelect
                                        v-model="t.blockedFilter.value"
                                        :items="verdictItems"
                                        class="w-32"
                                    />
                                    <UButton
                                        icon="i-lucide-x"
                                        variant="ghost"
                                        color="neutral"
                                        square
                                        @click="t.clearSelection()"
                                    />
                                </div>
                            </div>
                        </template>

                        <div class="p-4">
                            <UAlert
                                v-if="t.domainsError.value"
                                color="error"
                                variant="subtle"
                                icon="i-lucide-circle-x"
                                title="Failed to load domains"
                                :description="t.domainsError.value"
                                :actions="[
                                    {
                                        label: 'Retry',
                                        color: 'neutral',
                                        variant: 'outline',
                                        onClick: () => t.loadDomains(),
                                    },
                                ]"
                            />
                            <template v-else>
                                <UTable
                                    :loading="t.domainsLoading.value"
                                    empty="No domains for this device in range"
                                    :data="t.domains.value"
                                    :columns="domainColumns"
                                    :ui="{ root: 'relative' }"
                                />
                                <div
                                    v-if="t.domainsTotal.value > t.domainsPageSize"
                                    class="flex justify-center pt-4 border-t border-default mt-2"
                                >
                                    <UPagination
                                        :default-page="t.domainsPageIndex.value + 1"
                                        :items-per-page="t.domainsPageSize"
                                        :total="t.domainsTotal.value"
                                        @update:page="onDomainsPageChange"
                                    />
                                </div>
                            </template>
                        </div>
                    </UCard>
                </UContainer>
            </div>
        </template>

        <!-- ===================== TOP DOMAINS TAB ===================== -->
        <div v-else class="flex-1 min-h-0 overflow-auto">
            <UContainer class="flex flex-col gap-4 py-4">
                <div class="flex items-center justify-between gap-3 px-1">
                    <h2 class="text-lg font-medium">Top domains by traffic</h2>
                    <USelect
                        v-model="t.topBlockedFilter.value"
                        :items="verdictItems"
                        class="w-32"
                    />
                </div>

                <UAlert
                    v-if="t.topDomainsError.value"
                    color="error"
                    variant="subtle"
                    icon="i-lucide-circle-x"
                    title="Failed to load top domains"
                    :description="t.topDomainsError.value"
                    :actions="[
                        {
                            label: 'Retry',
                            color: 'neutral',
                            variant: 'outline',
                            onClick: () => t.loadTopDomains(),
                        },
                    ]"
                />

                <UCard v-else :ui="{ body: 'p-0 sm:p-0' }">
                    <UTable
                        :loading="t.topDomainsLoading.value"
                        sticky="header"
                        empty="No traffic recorded yet"
                        :data="t.topDomains.value"
                        :columns="topColumns"
                        :ui="{ root: 'relative' }"
                    />
                </UCard>
            </UContainer>
        </div>
    </div>
</template>
