const DATE_FORMAT: Intl.DateTimeFormatOptions = {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
};

export const formatDate = (input: string | number | Date | null | undefined): string => {
    if (!input) return "";
    const date = input instanceof Date ? input : new Date(input);
    if (Number.isNaN(date.getTime())) return "";
    return date.toLocaleString("en-US", DATE_FORMAT);
};
