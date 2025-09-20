import axios, {type AxiosInstance} from "axios";

const API_HOST = "http://localhost:8080"

export type DNSRecord = {
    id: number;
    active: boolean;
    createdAt: string;
    url: string;
}

type DNSRecordsResponse = {
    list: DNSRecord[];
    total: number
}

type DnsRecordsRequest = {
    limit: number;
    offset: number;
    filter: string;
}

type DNSRecordUpdateResponse = {
    record: DNSRecord;
}


type FilterStatusResponse = {
    status: boolean;
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
}


export const api = new Api();
