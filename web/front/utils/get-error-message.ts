import {isAxiosError} from "axios";

export const getErrorMessage = (error: unknown): string => {
    if (error instanceof Error) {
        return error.message;
    }
    if (isAxiosError(error)) {
        const response = error.response
        if (response && response.data && response.data.message) {
            return response.data.message;
        }
        return error.message;
    }
    return String(error);
}