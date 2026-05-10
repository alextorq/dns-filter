import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import { usePaginatedList, type PaginatedFetchParams } from "./use-paginated-list";

interface Item {
    id: number;
    name: string;
}

const sampleResponse = (overrides: Partial<{ list: Item[]; total: number }> = {}) => ({
    list: overrides.list ?? [{ id: 1, name: "alpha" }],
    total: overrides.total ?? 1,
});

let toastAdd: ReturnType<typeof vi.fn>;

beforeEach(() => {
    toastAdd = vi.fn();
    vi.stubGlobal("useToast", () => ({ add: toastAdd }));
});

afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
});

describe("usePaginatedList", () => {
    it("initializes with empty state and provided pageSize", () => {
        const fetcher = vi.fn(async () => sampleResponse());
        const list = usePaginatedList<Item>(fetcher, { pageSize: 25 });

        expect(list.data.value).toEqual([]);
        expect(list.filter.value).toBe("");
        expect(list.pagination.value).toEqual({ pageIndex: 0, pageSize: 25, total: 0 });
        expect(fetcher).not.toHaveBeenCalled();
    });

    it("refresh fetches with limit/offset/filter and writes data + total", async () => {
        const items = [
            { id: 1, name: "a" },
            { id: 2, name: "b" },
        ];
        const fetcher = vi.fn(async () => sampleResponse({ list: items, total: 42 }));
        const list = usePaginatedList<Item>(fetcher, { pageSize: 12 });

        await list.refresh();

        expect(fetcher).toHaveBeenCalledTimes(1);
        const params = fetcher.mock.calls[0]![0] as PaginatedFetchParams;
        expect(params.limit).toBe(12);
        expect(params.offset).toBe(0);
        expect(params.filter).toBe("");
        expect(params.signal).toBeInstanceOf(AbortSignal);

        expect(list.data.value).toEqual(items);
        expect(list.pagination.value.total).toBe(42);
    });

    it("changePage sets pageIndex and computes correct offset", async () => {
        const fetcher = vi.fn(async () => sampleResponse());
        const list = usePaginatedList<Item>(fetcher, { pageSize: 10 });

        await list.changePage(3);

        expect(list.pagination.value.pageIndex).toBe(2);
        const params = fetcher.mock.calls[0]![0] as PaginatedFetchParams;
        expect(params.offset).toBe(20);
        expect(params.limit).toBe(10);
    });

    it("filter change resets pageIndex to 0 and refetches (debounced)", async () => {
        vi.useFakeTimers();
        const fetcher = vi.fn(async () => sampleResponse());
        const list = usePaginatedList<Item>(fetcher, { pageSize: 5, debounceMs: 200 });

        // Move to page 3.
        await list.changePage(3);
        expect(list.pagination.value.pageIndex).toBe(2);
        expect(fetcher).toHaveBeenCalledTimes(1);

        // Change filter — should NOT fire immediately.
        list.filter.value = "abc";
        await nextTick();
        expect(fetcher).toHaveBeenCalledTimes(1);

        // After debounce window, refetch with reset pageIndex.
        await vi.advanceTimersByTimeAsync(200);
        // resetAndFetch is async — let microtasks settle.
        await vi.runAllTimersAsync();

        expect(fetcher).toHaveBeenCalledTimes(2);
        expect(list.pagination.value.pageIndex).toBe(0);
        const params = fetcher.mock.calls[1]![0] as PaginatedFetchParams;
        expect(params.filter).toBe("abc");
        expect(params.offset).toBe(0);
    });

    it("aborts the in-flight request when refresh is called again", async () => {
        const signals: AbortSignal[] = [];
        let resolveFirst!: (v: { list: Item[]; total: number }) => void;
        const fetcher = vi.fn((params: PaginatedFetchParams) => {
            signals.push(params.signal);
            if (signals.length === 1) {
                return new Promise<{ list: Item[]; total: number }>((resolve) => {
                    resolveFirst = resolve;
                });
            }
            return Promise.resolve(sampleResponse());
        });
        const list = usePaginatedList<Item>(fetcher);

        const first = list.refresh();
        // Second call must abort the first.
        const second = list.refresh();

        expect(signals[0]!.aborted).toBe(true);
        expect(signals[1]!.aborted).toBe(false);

        resolveFirst({ list: [], total: 0 });
        await first;
        await second;
    });

    it("shows an error toast on fetcher failure", async () => {
        const fetcher = vi.fn(async () => {
            throw new Error("boom");
        });
        const list = usePaginatedList<Item>(fetcher);

        await list.refresh();

        expect(toastAdd).toHaveBeenCalledTimes(1);
        const arg = toastAdd.mock.calls[0]![0] as { title: string; description: string };
        expect(arg.title).toBe("Error");
        expect(arg.description).toBe("boom");
    });

    it("does not toast on abort errors", async () => {
        const fetcher = vi.fn(async () => {
            const err = new DOMException("aborted", "AbortError");
            throw err;
        });
        const list = usePaginatedList<Item>(fetcher);

        await list.refresh();

        expect(toastAdd).not.toHaveBeenCalled();
    });

    it("treats null list/total in response as empty/zero", async () => {
        const fetcher = vi.fn(async () => ({ list: null, total: null }));
        const list = usePaginatedList<Item>(fetcher);

        await list.refresh();

        expect(list.data.value).toEqual([]);
        expect(list.pagination.value.total).toBe(0);
    });

    it("isLoading toggles around refresh", async () => {
        let resolve!: (v: { list: Item[]; total: number }) => void;
        const fetcher = vi.fn(
            () =>
                new Promise<{ list: Item[]; total: number }>((r) => {
                    resolve = r;
                }),
        );
        const list = usePaginatedList<Item>(fetcher);

        expect(list.isLoading.value).toBe(false);
        const pending = list.refresh();
        await nextTick();
        expect(list.isLoading.value).toBe(true);

        resolve({ list: [], total: 0 });
        await pending;
        expect(list.isLoading.value).toBe(false);
    });
});
