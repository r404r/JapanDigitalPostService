export type ApiStatus =
  | "ok"
  | "too_many_results"
  | "timeout"
  | "invalid_request"
  | "unauthorized"
  | "forbidden"
  | "not_found"
  | "rate_limited"
  | "internal_error"
  | "sync_running"
  | "unsupported_file"
  | "unzip_failed"
  | "csv_format_error";

export type Address = {
  zipcode?: string;
  jis_code?: string;
  prefecture?: string;
  prefecture_kana?: string;
  city?: string;
  city_kana?: string;
  town?: string;
  town_kana?: string;
};

export type SearchResult = {
  status: "ok" | "too_many_results" | "timeout";
  total_count: number;
  returned_count: number;
  truncated?: boolean;
  items: Address[];
};

export type SyncStatus = {
  total_addresses: number;
  running: boolean;
  last_success_at: string | null;
  last_type: SyncType | null;
};

export type SyncType = "auto" | "full" | "diff";

export type SyncRun = {
  id: string;
  type: SyncType;
  status: "running" | "success" | "failed";
  trigger?: "schedule" | "manual" | "upload";
  source_url?: string;
  file_checksum?: string;
  file_size?: number;
  diff_period?: string | null;
  rows_added?: number;
  rows_updated?: number;
  rows_deleted?: number;
  rows_total?: number;
  started_at: string;
  finished_at: string | null;
  error_message?: string | null;
};

export type TokenInfo = {
  id: string;
  name: string;
  prefix: string;
  scope: "read" | "admin";
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
};

export type CreatedToken = TokenInfo & {
  token: string;
};

export type AdminSetting<T> = {
  value: T;
  default: T;
  overridden: boolean;
};

export type AdminSettings = {
  download_max_retry: AdminSetting<number>;
  scrape_full_url: AdminSetting<string>;
};

export type AdminSettingsUpdate = {
  download_max_retry?: number;
  scrape_full_url?: string;
  reset_to_default?: Array<keyof AdminSettings>;
};

export type ApiError = {
  status: ApiStatus | "network_error";
  message: string;
  httpStatus?: number;
};

export class ApiClient {
  constructor(
    private readonly getToken: () => string,
    private readonly baseUrl = "/v1"
  ) {}

  searchAddresses(params: Record<string, string>) {
    const query = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value.trim()) {
        query.set(key, value.trim());
      }
    });
    query.set("limit", "20");
    return this.request<SearchResult>(`/addresses?${query.toString()}`);
  }

  getSyncStatus() {
    return this.request<SyncStatus>("/sync/status");
  }

  listSyncRuns() {
    return this.request<SyncRun[]>("/sync/runs?limit=100&offset=0");
  }

  triggerSync(type: SyncType) {
    return this.request<SyncRun>("/sync/trigger", {
      method: "POST",
      body: JSON.stringify({ type })
    });
  }

  getAdminSettings() {
    return this.request<AdminSettings>("/admin/settings");
  }

  updateAdminSettings(input: AdminSettingsUpdate) {
    return this.request<AdminSettings>("/admin/settings", {
      method: "PUT",
      body: JSON.stringify(input)
    });
  }

  uploadSync(file: File) {
    const body = new FormData();
    body.append("file", file);
    return this.request<SyncRun>("/sync/upload", {
      method: "POST",
      body
    });
  }

  listTokens() {
    return this.request<TokenInfo[]>("/tokens");
  }

  createToken(input: { name: string; scope: "read" | "admin" }) {
    return this.request<CreatedToken>("/tokens", {
      method: "POST",
      body: JSON.stringify(input)
    });
  }

  revokeToken(id: string) {
    return this.request<void>(`/tokens/${encodeURIComponent(id)}`, {
      method: "DELETE"
    });
  }

  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const token = this.getToken().trim();
    const headers = new Headers(init.headers);
    if (!headers.has("Content-Type") && init.body && !(init.body instanceof FormData)) {
      headers.set("Content-Type", "application/json");
    }
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }

    let response: Response;
    try {
      response = await fetch(`${this.baseUrl}${path}`, {
        ...init,
        headers
      });
    } catch {
      throw {
        status: "network_error",
        message: "API に接続できません。サーバー起動と VITE proxy 設定を確認してください。"
      } satisfies ApiError;
    }

    if (response.status === 204) {
      return undefined as T;
    }

    const contentType = response.headers.get("Content-Type") ?? "";
    const payload = contentType.includes("application/json")
      ? await response.json()
      : null;

    if (!response.ok) {
      throw normalizeError(payload, response.status);
    }

    return payload as T;
  }
}

export function normalizeError(payload: unknown, httpStatus?: number): ApiError {
  if (payload && typeof payload === "object") {
    const errorPayload = payload as { status?: string; message?: string };
    return {
      status: (errorPayload.status as ApiError["status"]) ?? "internal_error",
      message: errorPayload.message ?? defaultErrorMessage(errorPayload.status),
      httpStatus
    };
  }

  const status = fallbackStatus(httpStatus);
  return {
    status,
    message: defaultErrorMessage(status),
    httpStatus
  };
}

function fallbackStatus(httpStatus?: number): ApiError["status"] {
  switch (httpStatus) {
    case 400:
      return "invalid_request";
    case 401:
      return "unauthorized";
    case 403:
      return "forbidden";
    case 404:
      return "not_found";
    case 429:
      return "rate_limited";
    case 504:
      return "timeout";
    default:
      return "internal_error";
  }
}

export function defaultErrorMessage(status?: string) {
  switch (status) {
    case "timeout":
      return "検索がタイムアウトしました。条件を絞って再試行してください。";
    case "unauthorized":
      return "認証に失敗しました。Bearer token を確認してください。";
    case "forbidden":
      return "この操作には admin scope の token が必要です。";
    case "too_many_results":
      return "結果が多すぎます。条件を追加してください。";
    case "sync_running":
      return "同期はすでに実行中です。";
    case "unsupported_file":
      return "zip または csv ファイルを選択してください。";
    case "unzip_failed":
      return "zip ファイルを展開できませんでした。";
    case "csv_format_error":
      return "UTF-8 の utf_ken_all CSV のみ対応しています。";
    default:
      return "サービスエラーが発生しました。";
  }
}
