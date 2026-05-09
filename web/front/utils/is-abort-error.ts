import axios from "axios";

export const isAbortError = (error: unknown): boolean => {
    if (axios.isCancel(error)) return true;
    if (error instanceof DOMException && error.name === "AbortError") return true;
    if (
        typeof error === "object" &&
        error !== null &&
        "name" in error &&
        (error as { name?: string }).name === "CanceledError"
    ) {
        return true;
    }
    return false;
};
