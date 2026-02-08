import axios, {type AxiosInstance} from "axios";

export const API_HOST = import.meta.env.DEV 
    ? "http://localhost:8080/api" 
    : "/api"

export type DNSRecord = {
    id: number;
    active: boolean;
    createdAt: string;
    url: string;
    source: string;
}

export type SuggestBlock = {
    id: number;
    domain: string;
    reasons: string;
    score: number;
}

type DNSRecordsResponse = {
    list: DNSRecord[];
    total: number
}

type SuggestRecordsResponse = {
    list: SuggestBlock[];
    total: number
}

type DnsRecordsRequest = {
    limit: number;
    offset: number;
    filter: string;
    source: string;
}

type SuggestRecordsRequest = {
    limit: number;
    offset: number;
    filter: string;
    active: boolean | null;
}


type AddSuggestToBlockRequest = {
    id: number;
    domain: string;
}

type DNSRecordUpdateResponse = {
    record: DNSRecord;
}


type FilterStatusResponse = {
    status: boolean;
}

export type DomainBlockWithCount = {
    Domain: string;
    Count: number;
}

export type DomainsBlockGroup = {
    groups: DomainBlockWithCount[];
}

export type SyncRecord = {
    id: number;
    active: boolean;
    created_at: string;
    name: string;
}

type SyncRecordsResponse = {
    list: SyncRecord[];
    total: number
}

type SyncRecordsRequest = {
    limit: number;
    offset: number;
}

export class Api {
    private transport: AxiosInstance;

    constructor() {
        this.transport = axios.create({
            baseURL: API_HOST,
        })
    }

    async getAllDnsRecords(payload: DnsRecordsRequest, abortSignal: AbortSignal) {
        const {data} = await this.transport.post<DNSRecordsResponse>(`/dns-records`, payload, {signal: abortSignal});
        return data;
    }

    async getAllSuggestRecords(payload: SuggestRecordsRequest, abortSignal: AbortSignal) {
        const {data} = await this.transport.post<SuggestRecordsResponse>(`/suggest-to-block`, payload, {signal: abortSignal});
        return data;
    }

    async addSuggestToBlock(params: AddSuggestToBlockRequest) {
        const {data} = await this.transport.post<DNSRecordUpdateResponse>(`/suggest-to-block/add-to-block`, params);
        return data.record;
    }

    async changeDnsRecordStatus(id: number, active: boolean) {
        const {data} = await this.transport.post<DNSRecordUpdateResponse>(`/dns-records/update`, {
            active: active,
            id: id
        });
        return data.record;
    }


    async getFilterStatus() {
        const {data} = await this.transport.get<FilterStatusResponse>(`/filter/status`);
        return data.status;
    }

    async changeFilterStatus() {
        const {data} = await this.transport.post<FilterStatusResponse>(`/filter/change-status`);
        return data.status;
    }


    async createDomain(domain: string) {
        const {data} = await this.transport.post<DNSRecord>(`/dns-records/create`, {domain: domain});
        return data;
    }

    async getBlockDomainsGroups() {
        const {data} = await this.transport.post<DomainsBlockGroup>(`/events/block/amount-by-group`);
        return data;
    }

    async getBlockDomainsAmount() {
        const {data} = await this.transport.post<{amount: number}>(`/events/block/amount`);
        return data;
    }


    async changeLogLevel(level: string) {
        const {data} = await this.transport.post<{message: string}>(`/config/logger/change-level`, {logLevel: level});
    }

    async getLogLevel() {
        const {data} = await this.transport.post<{level: string}>(`/config/logger/get-level`);
        return data;
    }

    async getAllSyncRecords(payload: SyncRecordsRequest, abortSignal: AbortSignal) {
        const {data} = await this.transport.post<SyncRecordsResponse>(`/sources`, payload, {signal: abortSignal});
        return data;
    }

    async changeSyncRecordStatus(id: number, active: boolean) {
        const {data} = await this.transport.post<{record: SyncRecord}>(`/sources/change-status`, {
            active: active,
            id: id
        });
        return data.record;
    }
}


export const api = new Api();
