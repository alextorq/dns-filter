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

export enum DomainInspectVerdict {
  VerdictUnknown = "unknown",
  VerdictClean = "clean",
  VerdictSuspicious = "suspicious",
  VerdictMalicious = "malicious",
}

export enum DomainInspectCheckStatus {
  StatusOK = "ok",
  StatusError = "error",
  StatusSkipped = "skipped",
  StatusTimeout = "timeout",
}

export enum DbBlockListSource {
  SourceStevenBlack = "StevenBlack",
  SourceEasyList = "EasyList",
  SourceRuAdList = "RuAdList",
  SourceAdGuardRussian = "AdGuardRussian",
  SourceHaGeZiMulti = "HaGeZiMulti",
  SourceUser = "User",
  SourceSuggestedToBlock = "SuggestedToBlock",
}

export interface CollectSignalDescriptor {
  code?: string;
  description?: string;
  label?: string;
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

export interface DbClient {
  created_at?: string;
  deleted_at?: GormDeletedAt;
  filtered?: boolean;
  hostname?: string;
  id?: number;
  ip?: string;
  last_seen?: string;
  mac?: string;
  name?: string;
  token?: string;
  updated_at?: string;
  vendor?: string;
}

export interface DbDomainCount {
  count?: number;
  domain?: string;
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
  reasons?: DbSuggestBlockReason[];
  score?: number;
}

export interface DbSuggestBlockReason {
  code?: string;
  id?: number;
  match?: string;
}

export interface DiscoveryDevice {
  already_registered?: boolean;
  hostname?: string;
  ip?: string;
  mac?: string;
  source?: string;
  vendor?: string;
}

export interface DomainInspectCheckResult {
  details?: Record<string, any>;
  duration_ms?: number;
  error?: string;
  name?: string;
  status?: DomainInspectCheckStatus;
  verdict?: DomainInspectVerdict;
}

export interface DomainInspectInspectResult {
  checks?: DomainInspectCheckResult[];
  domain?: string;
  summary?: DomainInspectSummary;
}

export interface DomainInspectSummary {
  score?: number;
  verdict?: DomainInspectVerdict;
}

export interface GithubComAlextorqDnsFilterAuthWebErrorResponse {
  error?: string;
}

export interface GithubComAlextorqDnsFilterAuthWebStatusResponse {
  status?: string;
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

export interface GithubComAlextorqDnsFilterClientsWebStatusResponse {
  status?: string;
}

export interface GithubComAlextorqDnsFilterDomainInspectWebErrorResponse {
  message?: string;
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

export interface WebAddToBlockRequest {
  domain?: string;
  id?: number;
}

export interface WebBadRequestResponse {
  message?: string;
}

export interface WebChangeFilterRequest {
  filtered?: boolean;
  id?: number;
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

export interface WebClientResponse {
  client?: DbClient;
}

export interface WebCreateClientRequest {
  filtered?: boolean;
  hostname?: string;
  ip?: string;
  mac?: string;
  name?: string;
  token?: string;
  vendor?: string;
}

export interface WebDeleteClientRequest {
  id?: number;
}

export interface WebDiscoverResponse {
  devices?: DiscoveryDevice[];
  errors?: string[];
  total?: number;
}

export interface WebFilterStatusResponse {
  /**
   * PausedUntil is the unix-second deadline of an active pause, or 0 if no
   * pause is active. The frontend uses this absolute value to drive its
   * countdown without depending on server-supplied "seconds left".
   */
  paused_until?: number;
  status?: boolean;
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
  codes?: string[];
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

export interface WebGetSignalCodesResponse {
  list?: CollectSignalDescriptor[];
}

export interface WebListClientsResponse {
  list?: DbClient[];
  total?: number;
}

export interface WebLogLevelResponse {
  level?: string;
}

export interface WebLoginRequest {
  login: string;
  password: string;
}

export interface WebPauseFilterRequest {
  minutes?: number;
}

export interface WebUpdateClientRequest {
  hostname?: string;
  id?: number;
  name?: string;
  vendor?: string;
}

export interface WebUpdateConfigData {
  logLevel?: string;
}

export interface WebUpdateDnsRecordResponse {
  message?: string;
  record?: DbBlockList;
}

export interface WebUserResponse {
  id?: number;
  login?: string;
}
