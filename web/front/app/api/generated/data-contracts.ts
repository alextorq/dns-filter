/* eslint-disable */
/* tslint:disable */
// @ts-nocheck
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */

export enum DbBlockListSource {
  SourceStevenBlack = "StevenBlack",
  SourceEasyList = "EasyList",
  SourceUser = "User",
  SourceSuggestedToBlock = "SuggestedToBlock",
}

export interface CreateDomainRequestBody {
  domain?: string;
  source?: string;
}

export interface DbBlockDomainEvent {
  created_at?: string;
  domainId?: number;
  id?: number;
}

export interface DbBlockList {
  active?: boolean;
  /** One-to-Many */
  "blocked-events"?: DbBlockDomainEvent[];
  created_at?: string;
  deletedAt?: GormDeletedAt;
  id?: number;
  source?: string;
  updated_at?: string;
  url?: string;
}

export interface DbDomainCount {
  count?: number;
  domain?: string;
}

export interface DbExcludeClient {
  active?: boolean;
  created_at?: string;
  deletedAt?: GormDeletedAt;
  id?: number;
  updated_at?: string;
  user_id?: string;
}

export interface DbSource {
  active?: boolean;
  created_at?: string;
  deletedAt?: GormDeletedAt;
  id?: number;
  name?: DbBlockListSource;
  updated_at?: string;
}

export interface DbSuggestBlock {
  active?: boolean;
  domain?: string;
  id?: number;
  reasons?: string;
  score?: number;
}

export interface GithubComAlextorqDnsFilterBlockedDomainWebErrorResponse {
  message?: string;
}

export interface GithubComAlextorqDnsFilterBlockedDomainWebMessageResponse {
  message?: string;
}

export interface GithubComAlextorqDnsFilterClientsWebErrorResponse {
  error?: string;
}

export interface GithubComAlextorqDnsFilterLoggerWebMessageResponse {
  message?: string;
}

export interface GithubComAlextorqDnsFilterSourceWebErrorResponse {
  message?: string;
}

export interface GithubComAlextorqDnsFilterSuggestToBlockWebErrorResponse {
  message?: string;
}

export interface GithubComAlextorqDnsFilterSuggestToBlockWebMessageResponse {
  message?: string;
}

export interface GormDeletedAt {
  time?: string;
  /** Valid is true if Time is not NULL */
  valid?: boolean;
}

export interface UpdateDnsRecordUpdateBlockList {
  active?: boolean;
  id: number;
}

export interface WebAddClientRequest {
  user_id?: string;
}

export interface WebAddToBlockRequest {
  domain?: string;
  id?: number;
}

export interface WebBadRequestResponse {
  message?: string;
}

export interface WebChangeClientStatusRequest {
  id?: number;
  is_active?: boolean;
}

export interface WebChangeSourceActiveRequest {
  active?: boolean;
  id?: number;
}

export interface WebChangeSourceActiveResponse {
  message?: string;
  record?: DbSource;
}

export interface WebChangeSuggestStatusRequest {
  active?: boolean;
  id?: number;
}

export interface WebDeleteClientRequest {
  id?: number;
}

export interface WebFilterStatusResponse {
  status?: boolean;
}

export interface WebGetAllClientsResponse {
  list?: DbExcludeClient[];
  total?: number;
}

export interface WebGetAllDnsRecordsRequest {
  filter?: string;
  limit?: number;
  offset?: number;
  source?: string;
}

export interface WebGetAllDnsRecordsResponse {
  list?: DbBlockList[];
  total?: number;
}

export interface WebGetAllSourcesResponse {
  list?: DbSource[];
  total?: number;
}

export interface WebGetAllSuggestBlocksRequest {
  active?: boolean;
  filter?: string;
  limit?: number;
  offset?: number;
}

export interface WebGetAllSuggestBlocksResponse {
  list?: DbSuggestBlock[];
  total?: number;
}

export interface WebGetAmountByDomainResponse {
  groups?: DbDomainCount[];
}

export interface WebGetAmountResponse {
  amount?: number;
}

export interface WebLogLevelResponse {
  level?: string;
}

export interface WebStatusResponse {
  status?: string;
}

export interface WebUpdateConfigData {
  logLevel?: string;
}

export interface WebUpdateDnsRecordResponse {
  message?: string;
  record?: DbBlockList;
}
