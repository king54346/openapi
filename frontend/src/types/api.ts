export interface ApiResponse<T = unknown> {
  success: boolean
  message: string
  data: T
}

export interface PageData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface SetupStatus {
  initialized: boolean
  root_user: boolean
  version: string
}

export interface PublicStatus {
  version: string
  start_time: number
  setup: boolean
  server_address: string
  turnstile_check: boolean
  turnstile_site_key: string
  channel_affinity: boolean
  enable_batch_update: boolean
}

export interface StatusCheckItem {
  name: string
  success: boolean
  message?: string
}

export interface StatusTestResult {
  success: boolean
  items: StatusCheckItem[]
}

export interface User {
  id: number
  username: string
  display_name: string
  role: number
  status: number
  email?: string
  quota?: number
  used_quota?: number
  request_count?: number
  remark?: string
}

export interface Token {
  id: number
  user_id: number
  key: string
  name: string
  status: number
  created_time: number
  accessed_time: number
  expired_time: number
  remain_quota: number
  unlimited_quota: boolean
  used_quota: number
  model_limits_enabled: boolean
  model_limits: string
  model_limits_type: number
  allow_ips: string
  cross_group_retry: boolean
}

export interface TokenAccessTemplate {
  id: number
  user_id?: number
  name: string
  model_limits_enabled: boolean
  model_limits: string
  model_limits_type: number
  allow_ips?: string
  cross_group_retry?: boolean
  created_time?: number
  updated_time?: number
}

export interface TokenModelStat {
  model_name: string
  call_count: number
  calls?: number
  quota: number
  prompt_tokens: number
  completion_tokens: number
}

export interface TokenDailyStat {
  date: string
  call_count: number
  calls?: number
  quota: number
  prompt_tokens: number
  completion_tokens: number
}

export interface TokenAnalytics {
  token_id?: number
  token_name?: string
  range_days?: number
  summary: {
    total_calls: number
    total_quota: number
    total_prompt_tokens?: number
    total_completion_tokens?: number
    prompt_tokens?: number
    completion_tokens?: number
  }
  model_stats: TokenModelStat[]
  daily_stats?: TokenDailyStat[]
}

export interface ChannelInfo {
  is_multi_key: boolean
  multi_key_size: number
  multi_key_polling_index: number
  multi_key_mode: 'random' | 'polling' | ''
  multi_key_status_list?: Record<number, number>
}

export interface Channel {
  id: number
  type: number
  key?: string
  status: number
  name: string
  weight?: number
  created_time: number
  test_time: number
  response_time: number
  base_url?: string
  other?: string
  balance: number
  balance_updated_time: number
  models: string
  group?: string
  used_quota: number
  model_mapping?: string
  status_code_mapping?: string
  priority?: number
  auto_ban?: number
  other_info?: string
  tag?: string
  setting?: string
  param_override?: string
  header_override?: string
  remark?: string
  channel_info?: ChannelInfo
}

export interface ChannelPageData extends PageData<Channel> {
  type_counts?: Record<string, number>
}

export interface KeyStatusItem {
  index: number
  status: number
  disabled_time?: number
  reason?: string
  key_preview: string
}

export interface MultiKeyStatusResponse {
  keys: KeyStatusItem[]
  total: number
  page: number
  page_size: number
  total_pages: number
  enabled_count: number
  manual_disabled_count: number
  auto_disabled_count: number
}

export interface ChannelTestRecord {
  id: number
  channel_id: number
  channel_name: string
  channel_type: number
  channel_tag?: string
  model_name: string
  test_type: string
  status: number
  response_time_ms: number
  error_message?: string
  tested_at: number
}

export interface AffinityStats {
  enabled: boolean
  entries: number
  max_entries: number
  ttl_seconds: number
  hits: number
  misses: number
  records: number
  hit_rate: number
}

export interface AffinityUsageStat {
  channel_id: number
  count: number
}

export interface LogItem {
  id: number
  user_id: number
  created_at: number
  type: number
  content: string
  username: string
  token_name: string
  model_name: string
  quota: number
  prompt_tokens: number
  completion_tokens: number
  use_time: number
  is_stream: boolean
  channel: number
  channel_name?: string
  token_id: number
  ip: string
  request_id?: string
  other?: string
}

export interface LogStat {
  quota: number
  rpm: number
  tpm: number
}

export interface TaskItem {
  id: number
  created_at: number
  updated_at: number
  task_id: string
  platform: string
  user_id: number
  channel_id: number
  quota: number
  action: string
  status: string
  fail_reason: string
  submit_time: number
  start_time: number
  finish_time: number
  progress: string
  properties?: {
    input?: string
    upstream_model_name?: string
    origin_model_name?: string
  }
  data?: unknown
}

export interface CronJob {
  id: number
  name: string
  type: string
  cron: string
  params?: string
  enabled: boolean
  last_run_at?: number
  last_status?: string
  last_message?: string
  created_at?: number
  updated_at?: number
}

export interface CronJobRun {
  id: number
  job_id: number
  job_name?: string
  manual?: boolean
  status?: string
  success?: boolean
  duration_ms?: number
  latency_ms?: number
  message?: string
  started_at?: number
  created_at?: number
}

export interface BoundChannel {
  name: string
  type: number
}

export interface ModelMeta {
  id: number
  model_name: string
  description?: string
  icon?: string
  tags?: string
  vendor_id?: number
  endpoints?: string
  status: number
  sync_official: number
  name_rule: number
  created_time?: number
  updated_time?: number
  bound_channels?: BoundChannel[]
  matched_models?: string[]
  matched_count?: number
}

export interface ModelMetaPageData extends PageData<ModelMeta> {
  vendor_counts?: Record<string, number>
}

export interface VendorMeta {
  id: number
  name: string
  description?: string
  icon?: string
  status: number
  created_time?: number
  updated_time?: number
}

export interface PrefillGroup {
  id: number
  name: string
  type: string
  items: unknown
  description?: string
  created_time?: number
  updated_time?: number
}

export interface SyncPreviewResponse {
  create: ModelMeta[]
  update: ModelMeta[]
  total: number
}

export interface PerformanceStats {
  cache_stats?: {
    active_disk_files: number
    current_disk_usage_bytes: number
    active_memory_buffers: number
    current_memory_usage_bytes: number
    disk_cache_hits: number
    memory_cache_hits: number
    disk_cache_max_bytes: number
    disk_cache_threshold_bytes: number
  }
  memory_stats?: {
    alloc: number
    total_alloc: number
    sys: number
    num_gc: number
    num_goroutine: number
  }
  disk_cache_info?: {
    path: string
    exists: boolean
    file_count: number
    total_size: number
  }
  disk_space_info?: {
    total: number
    free: number
    used: number
    used_percent: number
  }
  config?: {
    disk_cache_path: string
    disk_cache_threshold_mb: number
    disk_cache_max_size_mb: number
    monitor_cpu_threshold: number
    monitor_memory_threshold: number
    monitor_disk_threshold: number
    disk_cache_enabled: boolean
    is_running_in_container: boolean
    monitor_enabled: boolean
  }
  admission_stats?: {
    enabled: boolean
    in_flight: number
    soft_limit: number
    hard_limit: number
    max_queue_size: number
    max_wait_ms: number
    priority_whitelist: string
    queued: number
    total_admitted: number
    total_queued: number
    total_rejected: number
    total_timeout: number
  }
  system_status?: {
    cpu_usage: number
    memory_usage: number
    disk_usage: number
  }
  ssrf_config?: {
    enabled: boolean
    allow_private_ip: boolean
    apply_ip_filter_for_domain: boolean
    domain_filter_mode: boolean
    ip_filter_mode: boolean
    domain_list: string[]
    ip_list: string[]
    allowed_ports: string[]
  }
}
