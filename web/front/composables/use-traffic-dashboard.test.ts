import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "~/api";
import { useTrafficDashboard } from "./use-traffic-dashboard";

const devicesResponse = {
    devices: [
        {
            client_kind: "mac",
            client_value: "aa:bb:cc:dd:ee:ff",
            current_ip: "192.168.0.10",
            vendor: "Apple, Inc.",
            allowed_count: 120,
            blocked_count: 5,
            last_seen: "2026-05-25T10:00:00Z",
        },
        {
            client_kind: "ip",
            client_value: "192.168.0.20",
            current_ip: "192.168.0.20",
            vendor: "",
            allowed_count: 7,
            blocked_count: 0,
            last_seen: "2026-05-24T08:00:00Z",
        },
    ],
};

const domainsResponse = {
    total: 2,
    list: [
        { domain: "ads.example.com", count: 42 },
        { domain: "cdn.example.com", count: 3 },
    ],
};

const topDomainsResponse = {
    list: [
        { domain: "telemetry.example.com", count: 999 },
        { domain: "tracker.example.net", count: 12 },
    ],
};

let devicesSpy: ReturnType<typeof vi.spyOn>;
let domainsSpy: ReturnType<typeof vi.spyOn>;
let topSpy: ReturnType<typeof vi.spyOn>;
let toastAdd: ReturnType<typeof vi.fn>;

beforeEach(() => {
    toastAdd = vi.fn();
    vi.stubGlobal("useToast", () => ({ add: toastAdd }));
    devicesSpy = vi.spyOn(api, "getTrafficDevices");
    domainsSpy = vi.spyOn(api, "getTrafficDeviceDomains");
    topSpy = vi.spyOn(api, "getTrafficTopDomains");
});

afterEach(() => {
    devicesSpy.mockRestore();
    domainsSpy.mockRestore();
    topSpy.mockRestore();
    vi.unstubAllGlobals();
});

describe("useTrafficDashboard — devices", () => {
    it("loads devices and clears the loading/error state on success", async () => {
        devicesSpy.mockResolvedValue(devicesResponse);
        const d = useTrafficDashboard();

        expect(d.devices.value).toEqual([]);
        expect(d.devicesLoading.value).toBe(false);

        await d.loadDevices();

        expect(devicesSpy).toHaveBeenCalledTimes(1);
        expect(d.devices.value).toEqual(devicesResponse.devices);
        expect(d.devicesLoading.value).toBe(false);
        expect(d.devicesError.value).toBeNull();
    });

    it("passes the date range to the devices request", async () => {
        devicesSpy.mockResolvedValue(devicesResponse);
        const d = useTrafficDashboard();
        d.from.value = "2026-05-01";
        d.to.value = "2026-05-25";

        await d.loadDevices();

        const arg = devicesSpy.mock.calls[0]![0] as { from?: string; to?: string };
        expect(arg.from).toBe("2026-05-01");
        expect(arg.to).toBe("2026-05-25");
    });

    it("sets a visible error and stops loading when the devices fetch rejects", async () => {
        devicesSpy.mockRejectedValue(new Error("devices boom"));
        const d = useTrafficDashboard();

        await d.loadDevices();

        expect(d.devicesError.value).toBe("devices boom");
        expect(d.devicesLoading.value).toBe(false);
        expect(d.devices.value).toEqual([]);
    });

    it("treats a null devices array as empty", async () => {
        devicesSpy.mockResolvedValue({ devices: null });
        const d = useTrafficDashboard();

        await d.loadDevices();

        expect(d.devices.value).toEqual([]);
        expect(d.devicesError.value).toBeNull();
    });

    it("does not set an error state when the devices fetch is aborted", async () => {
        devicesSpy.mockRejectedValue(new DOMException("aborted", "AbortError"));
        const d = useTrafficDashboard();

        await d.loadDevices();

        expect(d.devicesError.value).toBeNull();
        expect(d.devicesLoading.value).toBe(false);
    });
});

describe("useTrafficDashboard — device domains", () => {
    it("selecting a device loads its domains with kind/value and pagination", async () => {
        domainsSpy.mockResolvedValue(domainsResponse);
        const d = useTrafficDashboard();

        await d.selectDevice(devicesResponse.devices[0]!);

        expect(domainsSpy).toHaveBeenCalledTimes(1);
        const arg = domainsSpy.mock.calls[0]![0] as Record<string, unknown>;
        expect(arg.kind).toBe("mac");
        expect(arg.value).toBe("aa:bb:cc:dd:ee:ff");
        expect(arg.limit).toBe(50);
        expect(arg.offset).toBe(0);
        expect(d.domains.value).toEqual(domainsResponse.list);
        expect(d.domainsTotal.value).toBe(2);
        expect(d.domainsError.value).toBeNull();
    });

    it("applies the blocked filter when set", async () => {
        domainsSpy.mockResolvedValue(domainsResponse);
        const d = useTrafficDashboard();
        await d.selectDevice(devicesResponse.devices[0]!);
        domainsSpy.mockClear();

        d.blockedFilter.value = "blocked";
        await d.loadDomains();

        const arg = domainsSpy.mock.calls[0]![0] as Record<string, unknown>;
        expect(arg.blocked).toBe(true);
    });

    it("omits the blocked filter when set to all", async () => {
        domainsSpy.mockResolvedValue(domainsResponse);
        const d = useTrafficDashboard();
        await d.selectDevice(devicesResponse.devices[0]!);
        domainsSpy.mockClear();

        d.blockedFilter.value = "all";
        await d.loadDomains();

        const arg = domainsSpy.mock.calls[0]![0] as Record<string, unknown>;
        expect(arg.blocked).toBeUndefined();
    });

    it("changeDomainsPage advances the offset by limit", async () => {
        domainsSpy.mockResolvedValue(domainsResponse);
        const d = useTrafficDashboard();
        await d.selectDevice(devicesResponse.devices[0]!);
        domainsSpy.mockClear();

        await d.changeDomainsPage(3);

        const arg = domainsSpy.mock.calls[0]![0] as Record<string, unknown>;
        expect(arg.offset).toBe(100);
        expect(arg.limit).toBe(50);
    });

    it("sets a visible error when the domains fetch rejects", async () => {
        domainsSpy.mockRejectedValue(new Error("domains boom"));
        const d = useTrafficDashboard();

        await d.selectDevice(devicesResponse.devices[0]!);

        expect(d.domainsError.value).toBe("domains boom");
        expect(d.domainsLoading.value).toBe(false);
    });

    it("clears the selected device and skips fetching on clearSelection", async () => {
        domainsSpy.mockResolvedValue(domainsResponse);
        const d = useTrafficDashboard();
        await d.selectDevice(devicesResponse.devices[0]!);

        d.clearSelection();

        expect(d.selectedDevice.value).toBeNull();
        expect(d.domains.value).toEqual([]);
    });
});

describe("useTrafficDashboard — top domains", () => {
    it("loads top domains on success", async () => {
        topSpy.mockResolvedValue(topDomainsResponse);
        const d = useTrafficDashboard();

        await d.loadTopDomains();

        expect(topSpy).toHaveBeenCalledTimes(1);
        expect(d.topDomains.value).toEqual(topDomainsResponse.list);
        expect(d.topDomainsError.value).toBeNull();
    });

    it("passes the blocked filter to the top-domains request", async () => {
        topSpy.mockResolvedValue(topDomainsResponse);
        const d = useTrafficDashboard();
        d.topBlockedFilter.value = "allowed";

        await d.loadTopDomains();

        const arg = topSpy.mock.calls[0]![0] as { blocked?: boolean };
        expect(arg.blocked).toBe(false);
    });

    it("sets a visible error when the top-domains fetch rejects", async () => {
        topSpy.mockRejectedValue(new Error("top boom"));
        const d = useTrafficDashboard();

        await d.loadTopDomains();

        expect(d.topDomainsError.value).toBe("top boom");
        expect(d.topDomainsLoading.value).toBe(false);
        expect(d.topDomains.value).toEqual([]);
    });
});
