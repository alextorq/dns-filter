import { computed, ref, type Ref } from "vue";
import { api } from "~/api";
import type { WebDeviceDTO, WebDomainCountDTO } from "~/api/generated/data-contracts";
import { getErrorMessage } from "../utils/get-error-message";
import { isAbortError } from "../utils/is-abort-error";

// Verdict filter shared by the device-domains and top-domains views.
// "all" omits the `blocked` query param so the backend returns both.
export type VerdictFilter = "all" | "blocked" | "allowed";

const DOMAINS_PAGE_SIZE = 15;
const TOP_DOMAINS_LIMIT = 50;

const verdictToBlocked = (v: VerdictFilter): boolean | undefined => {
    if (v === "blocked") return true;
    if (v === "allowed") return false;
    return undefined;
};

const deviceKey = (d: WebDeviceDTO): string => `${d.client_kind ?? ""}:${d.client_value ?? ""}`;

/**
 * Data layer for the per-device traffic dashboard. Each independent fetch
 * (devices, the selected device's domains, top domains) owns its own
 * loading + error refs so the page can render a *visible error state* for
 * each one independently — a rejected fetch never leaves a permanent
 * skeleton. Aborted requests (superseded by a newer one) are swallowed.
 */
export const useTrafficDashboard = () => {
    // --- Device list ---
    const devices = ref([]) as Ref<WebDeviceDTO[]>;
    const devicesLoading = ref(false);
    const devicesError = ref<string | null>(null);
    let devicesController: AbortController | null = null;

    const loadDevices = async () => {
        if (devicesController) devicesController.abort();
        const ctrl = new AbortController();
        devicesController = ctrl;
        devicesLoading.value = true;
        devicesError.value = null;
        try {
            const res = await api.getTrafficDevices({}, ctrl.signal);
            devices.value = res.devices ?? [];
        } catch (error) {
            if (isAbortError(error)) return;
            devicesError.value = getErrorMessage(error);
            console.error("Failed to load traffic devices:", error);
        } finally {
            // Only the latest in-flight request owns the loading flag; a
            // superseded (aborted) request must not flip it off early.
            if (devicesController === ctrl) devicesLoading.value = false;
        }
    };

    // --- Selected device drill-down (domains) ---
    const selectedDevice = ref<WebDeviceDTO | null>(null);
    const domains = ref([]) as Ref<WebDomainCountDTO[]>;
    const domainsTotal = ref(0);
    const domainsLoading = ref(false);
    const domainsError = ref<string | null>(null);
    const blockedFilter = ref<VerdictFilter>("all");
    const domainsPageIndex = ref(0);
    let domainsController: AbortController | null = null;

    const selectedKey = computed(() =>
        selectedDevice.value ? deviceKey(selectedDevice.value) : null,
    );

    const loadDomains = async () => {
        const device = selectedDevice.value;
        if (!device) return;
        if (domainsController) domainsController.abort();
        const ctrl = new AbortController();
        domainsController = ctrl;
        domainsLoading.value = true;
        domainsError.value = null;
        try {
            const res = await api.getTrafficDeviceDomains(
                {
                    kind: device.client_kind ?? "",
                    value: device.client_value ?? "",
                    blocked: verdictToBlocked(blockedFilter.value),
                    limit: DOMAINS_PAGE_SIZE,
                    offset: domainsPageIndex.value * DOMAINS_PAGE_SIZE,
                },
                ctrl.signal,
            );
            domains.value = res.list ?? [];
            domainsTotal.value = res.total ?? 0;
        } catch (error) {
            if (isAbortError(error)) return;
            domainsError.value = getErrorMessage(error);
            console.error("Failed to load device domains:", error);
        } finally {
            // Only the latest in-flight request owns the loading flag; a
            // superseded (aborted) request must not flip it off early.
            if (domainsController === ctrl) domainsLoading.value = false;
        }
    };

    const selectDevice = async (device: WebDeviceDTO) => {
        selectedDevice.value = device;
        domainsPageIndex.value = 0;
        await loadDomains();
    };

    const clearSelection = () => {
        if (domainsController) domainsController.abort();
        selectedDevice.value = null;
        domains.value = [];
        domainsTotal.value = 0;
        domainsError.value = null;
        domainsPageIndex.value = 0;
    };

    const changeDomainsPage = async (page: number) => {
        domainsPageIndex.value = page - 1;
        await loadDomains();
    };

    const reloadDomainsFromStart = async () => {
        domainsPageIndex.value = 0;
        await loadDomains();
    };

    // --- Top domains ---
    const topDomains = ref([]) as Ref<WebDomainCountDTO[]>;
    const topDomainsLoading = ref(false);
    const topDomainsError = ref<string | null>(null);
    const topBlockedFilter = ref<VerdictFilter>("all");
    let topController: AbortController | null = null;

    const loadTopDomains = async () => {
        if (topController) topController.abort();
        const ctrl = new AbortController();
        topController = ctrl;
        topDomainsLoading.value = true;
        topDomainsError.value = null;
        try {
            const res = await api.getTrafficTopDomains(
                { blocked: verdictToBlocked(topBlockedFilter.value), limit: TOP_DOMAINS_LIMIT },
                ctrl.signal,
            );
            topDomains.value = res.list ?? [];
        } catch (error) {
            if (isAbortError(error)) return;
            topDomainsError.value = getErrorMessage(error);
            console.error("Failed to load top domains:", error);
        } finally {
            // Only the latest in-flight request owns the loading flag; a
            // superseded (aborted) request must not flip it off early.
            if (topController === ctrl) topDomainsLoading.value = false;
        }
    };

    // --- Headline totals (derived from the full device list, no extra fetch) ---
    // DeviceSummary returns every device (unpaginated), so summing its rows is the
    // grand total of observed queries — and it stays consistent with the top-domains
    // list, which is likewise all-time. heroMetric tracks the active verdict filter
    // so the big number, the ranked list and the filter all read as one view.
    const totalAllowed = computed(() =>
        devices.value.reduce((acc, d) => acc + (d.allowed_count ?? 0), 0),
    );
    const totalBlocked = computed(() =>
        devices.value.reduce((acc, d) => acc + (d.blocked_count ?? 0), 0),
    );
    const totalQueries = computed(() => totalAllowed.value + totalBlocked.value);
    const deviceCount = computed(() => devices.value.length);
    const heroMetric = computed(() => {
        if (topBlockedFilter.value === "blocked") return totalBlocked.value;
        if (topBlockedFilter.value === "allowed") return totalAllowed.value;
        return totalQueries.value;
    });

    return {
        // devices
        devices,
        devicesLoading,
        devicesError,
        loadDevices,
        // headline totals
        totalAllowed,
        totalBlocked,
        totalQueries,
        deviceCount,
        heroMetric,
        // drill-down
        selectedDevice,
        selectedKey,
        domains,
        domainsTotal,
        domainsLoading,
        domainsError,
        blockedFilter,
        domainsPageIndex,
        domainsPageSize: DOMAINS_PAGE_SIZE,
        selectDevice,
        clearSelection,
        loadDomains,
        reloadDomainsFromStart,
        changeDomainsPage,
        // top domains
        topDomains,
        topDomainsLoading,
        topDomainsError,
        topBlockedFilter,
        loadTopDomains,
        // helper for the page (key a device row)
        deviceKey,
    };
};
