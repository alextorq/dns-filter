<script setup lang="ts">
import type { TableColumn } from "@nuxt/ui";
import { api } from "~/api";
import type { DiscoveryDevice } from "~/api/generated/data-contracts";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";
import { isAbortError } from "~~/utils/is-abort-error";

const toast = useToast();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

let lastScanController: AbortController | null = null;

const devices = ref<DiscoveryDevice[]>([]);
const errors = ref<string[]>([]);
const hasScanned = ref(false);

// Track which devices are mid-add so we can disable the row's Add button.
// Keyed by IP because that's the canonical identifier in LAN mode.
const adding = reactive<Record<string, boolean>>({});

const emit = defineEmits<{
    (e: "added"): void;
}>();

const scan = async () => {
    if (lastScanController) lastScanController.abort();
    lastScanController = new AbortController();
    try {
        const response = await api.discoverNetwork(lastScanController.signal);
        devices.value = response.devices ?? [];
        errors.value = response.errors ?? [];
        hasScanned.value = true;
    } catch (error) {
        if (isAbortError(error)) return;
        toast.add({
            title: "Scan failed",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        console.error("Scan error:", error);
    }
};

const scanWithLoading = createLoadingRequest(scan);

const labelFor = (d: DiscoveryDevice) => d.hostname || d.vendor || d.ip || "";

const addAsClient = async (device: DiscoveryDevice) => {
    const ip = device.ip ?? "";
    if (!ip) return;
    adding[ip] = true;
    try {
        await api.createClient({
            ip,
            mac: device.mac ?? "",
            name: labelFor(device),
            hostname: device.hostname ?? "",
            vendor: device.vendor ?? "",
            // Discovered-and-added devices land as exclusions (filtered=false).
            // The driving use case is "I found my smart speaker, click Add to
            // bypass filter" — defaulting to excluded saves a second hop into
            // the My Clients tab to flip the toggle. Users wanting normal
            // filtering can flip it there.
            filtered: false,
        });
        toast.add({
            title: "Added",
            description: `${labelFor(device)} added to clients.`,
            duration: 3000,
        });
        // Mark locally so the button flips to "Registered" without refetching.
        const idx = devices.value.findIndex((d) => d.ip === ip);
        if (idx !== -1) {
            devices.value[idx] = { ...devices.value[idx], already_registered: true };
        }
        emit("added");
    } catch (error) {
        toast.add({
            title: "Error",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        console.error("Add error:", error);
    } finally {
        adding[ip] = false;
    }
};

const columns: TableColumn<DiscoveryDevice>[] = [
    {
        accessorKey: "ip",
        header: "IP",
        cell: ({ row }) => h("span", { class: "font-mono" }, row.original.ip ?? ""),
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
        accessorKey: "hostname",
        header: "Hostname",
        cell: ({ row }) => row.original.hostname || h("span", { class: "text-muted" }, "—"),
    },
    {
        accessorKey: "vendor",
        header: "Vendor",
        cell: ({ row }) => row.original.vendor || h("span", { class: "text-muted" }, "—"),
    },
    {
        accessorKey: "source",
        header: "Source",
        cell: ({ row }) => h("span", { class: "text-muted text-sm" }, row.original.source ?? ""),
    },
    {
        id: "actions",
        header: "",
        cell: ({ row }) => {
            const d = row.original;
            const ip = d.ip ?? "";
            if (d.already_registered) {
                return h("span", { class: "text-success text-sm" }, "Registered");
            }
            return h(
                "div",
                { class: "flex justify-end" },
                h(
                    resolveComponent("UButton"),
                    {
                        size: "sm",
                        icon: "i-lucide-plus",
                        loading: !!adding[ip],
                        disabled: !!adding[ip],
                        onClick: () => addAsClient(d),
                    },
                    () => "Add",
                ),
            );
        },
    },
];
</script>

<template>
    <div class="flex flex-col gap-4 pt-4">
        <div class="flex items-center justify-between gap-3 px-4">
            <div class="text-sm text-muted">
                <template v-if="!hasScanned">
                    Click <strong>Scan</strong> to enumerate devices on your local network.
                </template>
                <template v-else-if="devices.length === 0"> No devices found. </template>
                <template v-else>
                    Found {{ devices.length }} device{{ devices.length === 1 ? "" : "s" }}.
                </template>
            </div>
            <UButton
                size="lg"
                icon="i-lucide-radar"
                :loading="isLoading"
                :label="hasScanned ? 'Rescan' : 'Scan'"
                @click="scanWithLoading"
            />
        </div>

        <UAlert
            v-for="(err, i) in errors"
            :key="i"
            color="warning"
            variant="subtle"
            icon="i-lucide-triangle-alert"
            :title="'Partial scan failure'"
            :description="err"
            class="mx-4"
        />

        <UContainer v-if="devices.length > 0" class="px-0">
            <UTable
                :data="devices"
                :columns="columns"
                empty="No devices found"
                :ui="{ root: 'relative' }"
            />
        </UContainer>
    </div>
</template>
