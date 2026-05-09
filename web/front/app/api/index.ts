import { Api as GeneratedApi } from "./generated/Api";
import type {
    WebAddClientRequest,
    WebAddToBlockRequest,
    WebGetAllDnsRecordsRequest,
    WebGetAllSuggestBlocksRequest,
} from "./generated/data-contracts";

export const API_HOST = import.meta.env.DEV ? "http://localhost:8080/api" : "/api";

const CLIENT_BASE_URL = import.meta.env.DEV ? "http://localhost:8080" : "";

class Api {
    private client = new GeneratedApi({ baseURL: CLIENT_BASE_URL });

    getAllDnsRecords = (payload: WebGetAllDnsRecordsRequest, abortSignal: AbortSignal) =>
        this.client.dnsRecordsCreate(payload, { signal: abortSignal });

    getAllSuggestRecords = (payload: WebGetAllSuggestBlocksRequest, abortSignal: AbortSignal) =>
        this.client.suggestToBlockCreate(payload, { signal: abortSignal });

    addSuggestToBlock = (params: WebAddToBlockRequest) =>
        this.client.suggestToBlockAddToBlockCreate(params);

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
