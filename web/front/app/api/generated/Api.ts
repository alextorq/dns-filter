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

import type {
  CreateDomainRequestBody,
  DomainInspectInspectResult,
  GithubComAlextorqDnsFilterAuthWebErrorResponse,
  GithubComAlextorqDnsFilterAuthWebStatusResponse,
  GithubComAlextorqDnsFilterBlockedDomainWebErrorResponse,
  GithubComAlextorqDnsFilterBlockedDomainWebMessageResponse,
  GithubComAlextorqDnsFilterClientsWebErrorResponse,
  GithubComAlextorqDnsFilterClientsWebStatusResponse,
  GithubComAlextorqDnsFilterDomainInspectWebErrorResponse,
  GithubComAlextorqDnsFilterLoggerWebMessageResponse,
  GithubComAlextorqDnsFilterSourceWebErrorResponse,
  GithubComAlextorqDnsFilterSuggestToBlockWebErrorResponse,
  GithubComAlextorqDnsFilterSuggestToBlockWebMessageResponse,
  UpdateDnsRecordUpdateBlockList,
  WebAddToBlockRequest,
  WebBadRequestResponse,
  WebChangeFilterRequest,
  WebChangeSourceActiveRequest,
  WebChangeSourceActiveResponse,
  WebChangeSuggestStatusRequest,
  WebClearCacheResponse,
  WebClientResponse,
  WebCreateClientRequest,
  WebDeleteClientRequest,
  WebDiscoverResponse,
  WebFilterStatusResponse,
  WebGetAllDnsRecordsRequest,
  WebGetAllDnsRecordsResponse,
  WebGetAllSourcesResponse,
  WebGetAllSuggestBlocksRequest,
  WebGetAllSuggestBlocksResponse,
  WebGetAmountByDomainResponse,
  WebGetAmountResponse,
  WebGetSignalCodesResponse,
  WebListClientsResponse,
  WebLogLevelResponse,
  WebLoginRequest,
  WebPauseFilterRequest,
  WebUpdateClientRequest,
  WebUpdateConfigData,
  WebUpdateDnsRecordResponse,
  WebUserResponse,
} from "./data-contracts";
import type { RequestParams } from "./http-client";
import { ContentType, HttpClient } from "./http-client";

export class Api<
  SecurityDataType = unknown,
> extends HttpClient<SecurityDataType> {
  /**
   * No description
   *
   * @tags auth
   * @name AuthLoginCreate
   * @summary Login
   * @request POST:/api/auth/login
   */
  authLoginCreate = (body: WebLoginRequest, params: RequestParams = {}) =>
    this.request<
      WebUserResponse,
      GithubComAlextorqDnsFilterAuthWebErrorResponse
    >({
      path: `/api/auth/login`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags auth
   * @name AuthLogoutCreate
   * @summary Logout
   * @request POST:/api/auth/logout
   */
  authLogoutCreate = (params: RequestParams = {}) =>
    this.request<GithubComAlextorqDnsFilterAuthWebStatusResponse, any>({
      path: `/api/auth/logout`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags auth
   * @name AuthMeList
   * @summary Current user
   * @request GET:/api/auth/me
   */
  authMeList = (params: RequestParams = {}) =>
    this.request<
      WebUserResponse,
      GithubComAlextorqDnsFilterAuthWebErrorResponse
    >({
      path: `/api/auth/me`,
      method: "GET",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags clients
   * @name ClientsCreate
   * @summary List clients
   * @request POST:/api/clients
   */
  clientsCreate = (params: RequestParams = {}) =>
    this.request<
      WebListClientsResponse,
      GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/clients`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags clients
   * @name ClientsChangeFilterCreate
   * @summary Change client filter flag
   * @request POST:/api/clients/change-filter
   */
  clientsChangeFilterCreate = (
    body: WebChangeFilterRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      WebClientResponse,
      WebBadRequestResponse | GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/clients/change-filter`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags clients
   * @name ClientsCreateCreate
   * @summary Create client
   * @request POST:/api/clients/create
   */
  clientsCreateCreate = (
    body: WebCreateClientRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      WebClientResponse,
      WebBadRequestResponse | GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/clients/create`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags clients
   * @name ClientsDeleteCreate
   * @summary Delete client
   * @request POST:/api/clients/delete
   */
  clientsDeleteCreate = (
    body: WebDeleteClientRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      GithubComAlextorqDnsFilterClientsWebStatusResponse,
      WebBadRequestResponse | GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/clients/delete`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags clients
   * @name ClientsDiscoverCreate
   * @summary Scan LAN for devices
   * @request POST:/api/clients/discover
   */
  clientsDiscoverCreate = (params: RequestParams = {}) =>
    this.request<
      WebDiscoverResponse,
      GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/clients/discover`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags clients
   * @name ClientsUpdateCreate
   * @summary Update client metadata
   * @request POST:/api/clients/update
   */
  clientsUpdateCreate = (
    body: WebUpdateClientRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      WebClientResponse,
      WebBadRequestResponse | GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/clients/update`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags config
   * @name ConfigDbDownloadList
   * @summary Download database file
   * @request GET:/api/config/db/download
   */
  configDbDownloadList = (params: RequestParams = {}) =>
    this.request<Blob, any>({
      path: `/api/config/db/download`,
      method: "GET",
      ...params,
    });
  /**
   * No description
   *
   * @tags config
   * @name ConfigLoggerChangeLevelCreate
   * @summary Change log level
   * @request POST:/api/config/logger/change-level
   */
  configLoggerChangeLevelCreate = (
    body: WebUpdateConfigData,
    params: RequestParams = {},
  ) =>
    this.request<
      GithubComAlextorqDnsFilterLoggerWebMessageResponse,
      GithubComAlextorqDnsFilterLoggerWebMessageResponse
    >({
      path: `/api/config/logger/change-level`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags config
   * @name ConfigLoggerGetLevelCreate
   * @summary Get log level
   * @request POST:/api/config/logger/get-level
   */
  configLoggerGetLevelCreate = (params: RequestParams = {}) =>
    this.request<WebLogLevelResponse, any>({
      path: `/api/config/logger/get-level`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * @description Drops every entry from the in-memory DNS response cache. The next query for each name will be resolved upstream.
   *
   * @tags dns-cache
   * @name DnsCacheClearCreate
   * @summary Clear DNS response cache
   * @request POST:/api/dns-cache/clear
   */
  dnsCacheClearCreate = (params: RequestParams = {}) =>
    this.request<WebClearCacheResponse, any>({
      path: `/api/dns-cache/clear`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags dns-records
   * @name DnsRecordsCreate
   * @summary List blocked DNS records
   * @request POST:/api/dns-records
   */
  dnsRecordsCreate = (
    body: WebGetAllDnsRecordsRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      WebGetAllDnsRecordsResponse,
      GithubComAlextorqDnsFilterBlockedDomainWebErrorResponse
    >({
      path: `/api/dns-records`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags dns-records
   * @name DnsRecordsCreateCreate
   * @summary Create a blocked DNS record
   * @request POST:/api/dns-records/create
   */
  dnsRecordsCreateCreate = (
    body: CreateDomainRequestBody,
    params: RequestParams = {},
  ) =>
    this.request<
      GithubComAlextorqDnsFilterBlockedDomainWebMessageResponse,
      GithubComAlextorqDnsFilterBlockedDomainWebErrorResponse
    >({
      path: `/api/dns-records/create`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags dns-records
   * @name DnsRecordsUpdateCreate
   * @summary Update a blocked DNS record
   * @request POST:/api/dns-records/update
   */
  dnsRecordsUpdateCreate = (
    body: UpdateDnsRecordUpdateBlockList,
    params: RequestParams = {},
  ) =>
    this.request<
      WebUpdateDnsRecordResponse,
      GithubComAlextorqDnsFilterBlockedDomainWebErrorResponse
    >({
      path: `/api/dns-records/update`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags domain-inspect
   * @name DomainInspectList
   * @summary Inspect a domain with reputation/diagnostic checks
   * @request GET:/api/domain/inspect
   */
  domainInspectList = (
    query: {
      /** Domain to inspect (e.g. example.com) */
      domain: string;
    },
    params: RequestParams = {},
  ) =>
    this.request<
      DomainInspectInspectResult,
      GithubComAlextorqDnsFilterDomainInspectWebErrorResponse
    >({
      path: `/api/domain/inspect`,
      method: "GET",
      query: query,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags events
   * @name EventsBlockAmountCreate
   * @summary Total block events
   * @request POST:/api/events/block/amount
   */
  eventsBlockAmountCreate = (params: RequestParams = {}) =>
    this.request<WebGetAmountResponse, any>({
      path: `/api/events/block/amount`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags events
   * @name EventsBlockAmountByGroupCreate
   * @summary Block events grouped by domain
   * @request POST:/api/events/block/amount-by-group
   */
  eventsBlockAmountByGroupCreate = (params: RequestParams = {}) =>
    this.request<
      WebGetAmountByDomainResponse,
      GithubComAlextorqDnsFilterBlockedDomainWebErrorResponse
    >({
      path: `/api/events/block/amount-by-group`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags filter
   * @name FilterChangeStatusCreate
   * @summary Toggle the DNS filter
   * @request POST:/api/filter/change-status
   */
  filterChangeStatusCreate = (params: RequestParams = {}) =>
    this.request<WebFilterStatusResponse, any>({
      path: `/api/filter/change-status`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags filter
   * @name FilterPauseCreate
   * @summary Pause the DNS filter for N minutes
   * @request POST:/api/filter/pause
   */
  filterPauseCreate = (
    request: WebPauseFilterRequest,
    params: RequestParams = {},
  ) =>
    this.request<WebFilterStatusResponse, Record<string, string>>({
      path: `/api/filter/pause`,
      method: "POST",
      body: request,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags filter
   * @name FilterResumeCreate
   * @summary Resume the DNS filter (clear pause)
   * @request POST:/api/filter/resume
   */
  filterResumeCreate = (params: RequestParams = {}) =>
    this.request<WebFilterStatusResponse, any>({
      path: `/api/filter/resume`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags filter
   * @name FilterStatusList
   * @summary Get filter status
   * @request GET:/api/filter/status
   */
  filterStatusList = (params: RequestParams = {}) =>
    this.request<WebFilterStatusResponse, any>({
      path: `/api/filter/status`,
      method: "GET",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags sources
   * @name SourcesCreate
   * @summary List block-list sources
   * @request POST:/api/sources
   */
  sourcesCreate = (params: RequestParams = {}) =>
    this.request<
      WebGetAllSourcesResponse,
      GithubComAlextorqDnsFilterSourceWebErrorResponse
    >({
      path: `/api/sources`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags sources
   * @name SourcesChangeStatusCreate
   * @summary Toggle a block-list source
   * @request POST:/api/sources/change-status
   */
  sourcesChangeStatusCreate = (
    body: WebChangeSourceActiveRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      WebChangeSourceActiveResponse,
      GithubComAlextorqDnsFilterSourceWebErrorResponse
    >({
      path: `/api/sources/change-status`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags suggest-to-block
   * @name SuggestToBlockCreate
   * @summary List suggested-to-block domains
   * @request POST:/api/suggest-to-block
   */
  suggestToBlockCreate = (
    body: WebGetAllSuggestBlocksRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      WebGetAllSuggestBlocksResponse,
      GithubComAlextorqDnsFilterSuggestToBlockWebErrorResponse
    >({
      path: `/api/suggest-to-block`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags suggest-to-block
   * @name SuggestToBlockAddToBlockCreate
   * @summary Promote suggestion to block list
   * @request POST:/api/suggest-to-block/add-to-block
   */
  suggestToBlockAddToBlockCreate = (
    body: WebAddToBlockRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      GithubComAlextorqDnsFilterSuggestToBlockWebMessageResponse,
      GithubComAlextorqDnsFilterSuggestToBlockWebErrorResponse
    >({
      path: `/api/suggest-to-block/add-to-block`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags suggest-to-block
   * @name SuggestToBlockChangeStatusCreate
   * @summary Toggle suggestion active state
   * @request POST:/api/suggest-to-block/change-status
   */
  suggestToBlockChangeStatusCreate = (
    body: WebChangeSuggestStatusRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      GithubComAlextorqDnsFilterSuggestToBlockWebMessageResponse,
      GithubComAlextorqDnsFilterSuggestToBlockWebErrorResponse
    >({
      path: `/api/suggest-to-block/change-status`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags suggest-to-block
   * @name SuggestToBlockCodesList
   * @summary List reason codes
   * @request GET:/api/suggest-to-block/codes
   */
  suggestToBlockCodesList = (params: RequestParams = {}) =>
    this.request<WebGetSignalCodesResponse, any>({
      path: `/api/suggest-to-block/codes`,
      method: "GET",
      format: "json",
      ...params,
    });
}
