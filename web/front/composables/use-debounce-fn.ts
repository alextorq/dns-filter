export const useDebounceFn = <T extends (...args: unknown[]) => unknown>(fn: T, delay = 300) => {
    let timer: ReturnType<typeof setTimeout> | null = null;

    const debounced = (...args: Parameters<T>) => {
        if (timer) clearTimeout(timer);
        timer = setTimeout(() => {
            timer = null;
            fn(...args);
        }, delay);
    };

    const cancel = () => {
        if (timer) {
            clearTimeout(timer);
            timer = null;
        }
    };

    return { debounced, cancel };
};
