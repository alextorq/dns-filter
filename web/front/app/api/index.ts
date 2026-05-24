import { Api as GeneratedApi } from "./generated/Api";
import type {
    WebAddToBlockRequest,
    WebCreateClientRequest,
    WebGetAllDnsRecordsRequest,
    WebGetAllSuggestBlocksRequest,
    WebUpdateClientRequest,
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
        return this.client.filterStatusList();
    };

    changeFilterStatus = async () => {
        return this.client.filterChangeStatusCreate();
    };

    pauseFilter = async (minutes: number) => {
        return this.client.filterPauseCreate({ minutes });
    };

    resumeFilter = async () => {
        return this.client.filterResumeCreate();
    };

    createDomain = (domain: string) => this.client.dnsRecordsCreateCreate({ domain });

    getBlockDomainsGroups = () => this.client.eventsBlockAmountByGroupCreate();

    getBlockDomainsAmount = () => this.client.eventsBlockAmountCreate();

    // Runtime settings persisted in the DB (log level, DoH upstream, cache
    // params). listSettings returns each setting's effective value plus the
    // metadata the UI needs to render a typed editor.
    listSettings = () => this.client.settingsList();

    updateSetting = (key: string, value: string) => this.client.settingsUpdate(key, { value });

    resetSetting = (key: string) => this.client.settingsDelete(key);

    getAllSyncRecords = (abortSignal: AbortSignal) =>
        this.client.sourcesCreate({ signal: abortSignal });

    changeSyncRecordStatus = async (id: number, active: boolean) => {
        const data = await this.client.sourcesChangeStatusCreate({ id, active });
        return data.record;
    };

    getAllClients = (abortSignal: AbortSignal) =>
        this.client.clientsCreate({ signal: abortSignal });

    createClient = (payload: WebCreateClientRequest) => this.client.clientsCreateCreate(payload);

    updateClient = (payload: WebUpdateClientRequest) => this.client.clientsUpdateCreate(payload);

    changeClientFilter = (id: number, filtered: boolean) =>
        this.client.clientsChangeFilterCreate({ id, filtered });

    deleteClient = (id: number) => this.client.clientsDeleteCreate({ id });

    discoverNetwork = (abortSignal: AbortSignal) =>
        this.client.clientsDiscoverCreate({ signal: abortSignal });

    inspectDomain = (domain: string, abortSignal: AbortSignal) =>
        this.client.domainInspectList({ domain }, { signal: abortSignal });

    clearDnsCache = () => this.client.dnsCacheClearCreate();
}

export const api = new Api();
