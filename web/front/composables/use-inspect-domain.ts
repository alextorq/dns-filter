import { ref } from "vue";
import { api } from "~/api";
import type { DomainInspectInspectResult } from "~/api/generated/data-contracts";
import { getErrorMessage } from "../utils/get-error-message";
import { isAbortError } from "../utils/is-abort-error";

// useInspectDomain owns the lifecycle of a single inspect call: it aborts the
// previous in-flight request on a new run, surfaces a visible error state
// (required — the API failure UX must never stay in permanent loading), and
// resets the result when the user starts a fresh inspection so stale data
// from the previous domain doesn't briefly flash on screen.
export const useInspectDomain = () => {
    const result = ref<DomainInspectInspectResult | null>(null);
    const isLoading = ref(false);
    const errorMessage = ref<string>("");

    let controller: AbortController | null = null;

    const cancel = () => {
        controller?.abort();
        controller = null;
    };

    const run = async (domain: string) => {
        const trimmed = domain.trim().toLowerCase();
        if (!trimmed) {
            errorMessage.value = "Domain is required.";
            return;
        }

        cancel();
        controller = new AbortController();
        isLoading.value = true;
        errorMessage.value = "";
        result.value = null;

        try {
            const response = await api.inspectDomain(trimmed, controller.signal);
            result.value = response;
        } catch (e) {
            if (isAbortError(e)) return;
            errorMessage.value = getErrorMessage(e);
        } finally {
            isLoading.value = false;
        }
    };

    const reset = () => {
        cancel();
        result.value = null;
        errorMessage.value = "";
        isLoading.value = false;
    };

    return { result, isLoading, errorMessage, run, cancel, reset };
};
