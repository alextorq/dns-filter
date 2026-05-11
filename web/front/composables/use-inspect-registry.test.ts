import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import { api } from "~/api";
import { DomainInspectCheckStatus, DomainInspectVerdict } from "~/api/generated/data-contracts";
import { __clearInspectRegistry, useInspectRegistry } from "./use-inspect-registry";

const mockResult = {
    domain: "evil.com",
    summary: { score: 95, verdict: DomainInspectVerdict.VerdictMalicious },
    checks: [
        {
            name: "google-safe-browsing",
            status: DomainInspectCheckStatus.StatusOK,
            verdict: DomainInspectVerdict.VerdictMalicious,
        },
    ],
};

const flushAsync = async () => {
    // runOne awaits one network call, so two ticks are enough to drain the
    // microtasks queued by the mocked promise.
    await nextTick();
    await nextTick();
};

let inspectSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
    __clearInspectRegistry();
    inspectSpy = vi.spyOn(api, "inspectDomain");
});

afterEach(() => {
    inspectSpy.mockRestore();
});

describe("useInspectRegistry", () => {
    it("scans a registered domain and stores the result", async () => {
        inspectSpy.mockResolvedValue(mockResult);
        const { register, get } = useInspectRegistry();

        register("evil.com");
        expect(get("evil.com")?.status).toBe("loading");

        await flushAsync();

        expect(get("evil.com")?.status).toBe("done");
        expect(get("evil.com")?.result).toEqual(mockResult);
    });

    it("records an error message when the scan fails", async () => {
        inspectSpy.mockRejectedValue(new Error("boom"));
        const { register, get } = useInspectRegistry();

        register("bad.example");
        await flushAsync();

        expect(get("bad.example")?.status).toBe("error");
        expect(get("bad.example")?.error).toBe("boom");
    });

    it("deduplicates concurrent registrations for the same domain", async () => {
        inspectSpy.mockResolvedValue(mockResult);
        const { register } = useInspectRegistry();

        register("evil.com");
        register("evil.com");
        register("evil.com");
        await flushAsync();

        expect(inspectSpy).toHaveBeenCalledTimes(1);
    });

    it("normalizes the domain key (trim + lowercase)", async () => {
        inspectSpy.mockResolvedValue(mockResult);
        const { register, get } = useInspectRegistry();

        register("  Evil.COM  ");
        await flushAsync();

        expect(get("evil.com")?.status).toBe("done");
        expect(inspectSpy).toHaveBeenCalledWith("evil.com", expect.any(AbortSignal));
    });

    it("does not re-fetch a domain that already has a result", async () => {
        inspectSpy.mockResolvedValue(mockResult);
        const { register } = useInspectRegistry();

        register("evil.com");
        await flushAsync();
        register("evil.com");
        await flushAsync();

        expect(inspectSpy).toHaveBeenCalledTimes(1);
    });

    it("ignores empty domain inputs", () => {
        const { register, get } = useInspectRegistry();

        register("");
        register("   ");

        expect(get("")).toBeUndefined();
        expect(inspectSpy).not.toHaveBeenCalled();
    });

    it("respects MAX_CONCURRENT by queueing extra domains", async () => {
        let resolveCount = 0;
        const resolvers: Array<(v: typeof mockResult) => void> = [];
        inspectSpy.mockImplementation(
            () =>
                new Promise((resolve) => {
                    resolvers.push(resolve);
                    resolveCount++;
                }),
        );
        const { register, get } = useInspectRegistry();

        register("a.com");
        register("b.com");
        register("c.com");
        await nextTick();

        // Only MAX_CONCURRENT (=2) requests should be in flight; the third is queued.
        expect(resolveCount).toBe(2);
        expect(get("a.com")?.status).toBe("loading");
        expect(get("b.com")?.status).toBe("loading");
        expect(get("c.com")?.status).toBe("idle");

        resolvers[0]!(mockResult);
        await flushAsync();

        // Slot freed → third request picked up.
        expect(resolveCount).toBe(3);
        expect(get("c.com")?.status).toBe("loading");

        resolvers[1]!(mockResult);
        resolvers[2]!(mockResult);
        await flushAsync();
    });
});
