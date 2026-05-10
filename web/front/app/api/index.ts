import { Api as GeneratedApi } from "./generated/Api";
import type {
    WebAddClientRequest,
    WebAddToBlockRequest,
    WebGetAllDnsRecordsRequest,
    WebGetAllSuggestBlocksRequest,
} from "./generated/data-contracts";
import { useAuth } from "~~/composables/use-auth";

// Nitro devProxy proxies /api to the backend, so we can always use relative URLs
// — cookies are then treated as same-origin and SameSite=Lax just works.
export const API_HOST = "/api";

const CLIENT_BASE_URL = "";

export type CurrentUser = { id: number; login: string };

class Api {
    private client = new GeneratedApi({
        baseURL: CLIENT_BASE_URL,
        withCredentials: true,
    });

    constructor() {
        this.client.instance.interceptors.response.use(
            (response) => response,
            (error) => {
                const url: string = error?.config?.url ?? "";
                const isAuthProbe = url.includes("/api/auth/");
                if (error?.response?.status === 401 && !isAuthProbe) {
                    void this.handleUnauthenticated();
                }
                return Promise.reject(error);
            },
        );
    }

    private handleUnauthenticated = async () => {
        if (typeof window === "undefined") return;
        if (window.location.pathname.startsWith("/auth")) return;
        const nuxtApp = useNuxtApp();
        await nuxtApp.runWithContext(() => {
            const { setUnauthenticated } = useAuth();
            setUnauthenticated();
            return navigateTo("/auth");
        });
    };

    login = async (login: string, password: string): Promise<CurrentUser> => {
        const res = await this.client.instance.post<CurrentUser>("/api/auth/login", {
            login,
            password,
        });
        return res.data;
    };

    logout = async () => {
        await this.client.instance.post("/api/auth/logout");
    };

    me = async (): Promise<CurrentUser> => {
        const res = await this.client.instance.get<CurrentUser>("/api/auth/me");
        return res.data;
    };

    getAllDnsRecords = (payload: WebGetAllDnsRecordsRequest, abortSignal: AbortSignal) =>
        this.client.dnsRecordsCreate(payload, { signal: abortSignal });

    getAllSuggestRecords = (payload: WebGetAllSuggestBlocksRequest, abortSignal: AbortSignal) =>
        this.client.suggestToBlockCreate(payload, { signal: abortSignal });

    addSuggestToBlock = (params: WebAddToBlockRequest) =>
        this.client.suggestToBlockAddToBlockCreate(params);

    getSuggestSignalCodes = (abortSignal: AbortSignal) =>
        this.client.suggestToBlockCodesList({ signal: abortSignal });

    changeDnsRecordStatus = async (id: number, active: boolean) => {
        const data = await this.client.dnsRecordsUpdateCreate({ id, active });
        return data.record;
    };

    getFilterStatus = async () => {
        const data = await this.client.filterStatusList();
        return data.status;
    };

    changeFilterStatus = async () => {
        const data = await this.client.filterChangeStatusCreate();
        return data.status;
    };

    createDomain = (domain: string) => this.client.dnsRecordsCreateCreate({ domain });

    getBlockDomainsGroups = () => this.client.eventsBlockAmountByGroupCreate();

    getBlockDomainsAmount = () => this.client.eventsBlockAmountCreate();

    changeLogLevel = async (level: string) => {
        await this.client.configLoggerChangeLevelCreate({ logLevel: level });
    };

    getLogLevel = () => this.client.configLoggerGetLevelCreate();

    getAllSyncRecords = (abortSignal: AbortSignal) =>
        this.client.sourcesCreate({ signal: abortSignal });

    changeSyncRecordStatus = async (id: number, active: boolean) => {
        const data = await this.client.sourcesChangeStatusCreate({ id, active });
        return data.record;
    };

    getAllExcludeClients = (abortSignal: AbortSignal) =>
        this.client.excludeClientsCreate({ signal: abortSignal });

    addExcludeClient = (payload: WebAddClientRequest) =>
        this.client.excludeClientsAddCreate(payload);

    changeClientStatus = (id: number, active: boolean) =>
        this.client.excludeClientsChangeStatusCreate({ id, is_active: active });

    deleteClient = (id: number) => this.client.excludeClientsDeleteCreate({ id });
}

export const api = new Api();
