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

import type {
  CreateDomainRequestBody,
  GithubComAlextorqDnsFilterBlockedDomainWebErrorResponse,
  GithubComAlextorqDnsFilterBlockedDomainWebMessageResponse,
  GithubComAlextorqDnsFilterClientsWebErrorResponse,
  GithubComAlextorqDnsFilterLoggerWebMessageResponse,
  GithubComAlextorqDnsFilterSourceWebErrorResponse,
  GithubComAlextorqDnsFilterSuggestToBlockWebErrorResponse,
  GithubComAlextorqDnsFilterSuggestToBlockWebMessageResponse,
  UpdateDnsRecordUpdateBlockList,
  WebAddClientRequest,
  WebAddToBlockRequest,
  WebBadRequestResponse,
  WebChangeClientStatusRequest,
  WebChangeSourceActiveRequest,
  WebChangeSourceActiveResponse,
  WebChangeSuggestStatusRequest,
  WebDeleteClientRequest,
  WebFilterStatusResponse,
  WebGetAllClientsResponse,
  WebGetAllDnsRecordsRequest,
  WebGetAllDnsRecordsResponse,
  WebGetAllSourcesResponse,
  WebGetAllSuggestBlocksRequest,
  WebGetAllSuggestBlocksResponse,
  WebGetAmountByDomainResponse,
  WebGetAmountResponse,
  WebLogLevelResponse,
  WebStatusResponse,
  WebUpdateConfigData,
  WebUpdateDnsRecordResponse,
} from "./data-contracts";
import { ContentType, HttpClient } from "./http-client";
import type { RequestParams } from "./http-client";

export class Api<
  SecurityDataType = unknown,
> extends HttpClient<SecurityDataType> {
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
   * @tags exclude-clients
   * @name ExcludeClientsCreate
   * @summary List exclude clients
   * @request POST:/api/exclude-clients
   */
  excludeClientsCreate = (params: RequestParams = {}) =>
    this.request<
      WebGetAllClientsResponse,
      GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/exclude-clients`,
      method: "POST",
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags exclude-clients
   * @name ExcludeClientsAddCreate
   * @summary Add exclude client
   * @request POST:/api/exclude-clients/add
   */
  excludeClientsAddCreate = (
    body: WebAddClientRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      WebStatusResponse,
      WebBadRequestResponse | GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/exclude-clients/add`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags exclude-clients
   * @name ExcludeClientsChangeStatusCreate
   * @summary Change exclude-client active state
   * @request POST:/api/exclude-clients/change-status
   */
  excludeClientsChangeStatusCreate = (
    body: WebChangeClientStatusRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      WebStatusResponse,
      WebBadRequestResponse | GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/exclude-clients/change-status`,
      method: "POST",
      body: body,
      type: ContentType.Json,
      format: "json",
      ...params,
    });
  /**
   * No description
   *
   * @tags exclude-clients
   * @name ExcludeClientsDeleteCreate
   * @summary Delete exclude client
   * @request POST:/api/exclude-clients/delete
   */
  excludeClientsDeleteCreate = (
    body: WebDeleteClientRequest,
    params: RequestParams = {},
  ) =>
    this.request<
      WebStatusResponse,
      WebBadRequestResponse | GithubComAlextorqDnsFilterClientsWebErrorResponse
    >({
      path: `/api/exclude-clients/delete`,
      method: "POST",
      body: body,
      type: ContentType.Json,
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
}
