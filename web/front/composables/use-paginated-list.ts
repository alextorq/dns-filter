import { ref, watch, type Ref } from "vue";
import { useComponentStatusWithLoading } from "./use-component-status-with-loading";
import { useDebounceFn } from "./use-debounce-fn";
import { getErrorMessage } from "../utils/get-error-message";
import { isAbortError } from "../utils/is-abort-error";

export interface PaginatedFetchParams {
    limit: number;
    offset: number;
    filter: string;
    signal: AbortSignal;
}

export interface PaginatedResponse<T> {
    list?: T[] | null;
    total?: number | null;
}

export interface UsePaginatedListOptions {
    pageSize?: number;
    debounceMs?: number;
}

export const usePaginatedList = <T>(
    fetcher: (params: PaginatedFetchParams) => Promise<PaginatedResponse<T>>,
    options: UsePaginatedListOptions = {},
) => {
    const { pageSize = 12, debounceMs = 300 } = options;

    const toast = useToast();

    const data = ref([]) as Ref<T[]>;
    const filter = ref("");
    const pagination = ref({
        pageIndex: 0,
        pageSize,
        total: 0,
    });

    const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

    let lastFetchController: AbortController | null = null;

    const fetchData = async () => {
        try {
            if (lastFetchController) lastFetchController.abort();
            lastFetchController = new AbortController();
            const response = await fetcher({
                limit: pagination.value.pageSize,
                offset: pagination.value.pageIndex * pagination.value.pageSize || 0,
                filter: filter.value,
                signal: lastFetchController.signal,
            });

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

    const refresh = createLoadingRequest(fetchData);

    const resetAndFetch = async () => {
        pagination.value.pageIndex = 0;
        await refresh();
    };

    const { debounced: debouncedReset } = useDebounceFn(resetAndFetch, debounceMs);
    watch(filter, () => debouncedReset());

    const changePage = async (page: number) => {
        pagination.value.pageIndex = page - 1;
        await refresh();
    };

    return {
        data,
        filter,
        pagination,
        isLoading,
        refresh,
        resetAndFetch,
        changePage,
    };
};
