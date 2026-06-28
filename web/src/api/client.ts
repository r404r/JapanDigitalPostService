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
  rows_skipped?: number;
  rows_total?: number;
  started_at: string;
  finished_at: string | null;
  error_message?: string | null;
};

export type SyncSkippedRow = {
  id: number;
  run_id: string;
  source_type: "full" | "add" | "upload";
  line_number: number;
  zipcode?: string;
  jis_code?: string;
  prefecture?: string;
  city?: string;
  town?: string;
  town_kana?: string;
  reason?: string;
  pattern?: string;
  raw_record_json?: string;
  created_at?: string;
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
  town_skip_regex: AdminSetting<string>;
};

export type AdminSettingsUpdate = {
  download_max_retry?: number;
  scrape_full_url?: string;
  town_skip_regex?: string;
  reset_to_default?: Array<keyof AdminSettings>;
};

export type ApiError = {
  status: ApiStatus | "network_error";
  message: string;
  httpStatus?: number;
};

type PayloadEnvelope = {
  enc?: unknown;
  nonce?: unknown;
  ciphertext?: unknown;
};

const payloadEncryptionHeader = "X-Payload-Encryption";
const payloadEncryptionAlgo = "AES-256-GCM";
const payloadEncryptionKeyBytes = 32;

export class ApiClient {
  constructor(
    private readonly getToken: () => string,
    private readonly baseUrl = "/v1",
    private readonly getPayloadEncryptionKey: () => string = () => ""
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

  listSyncSkippedRows(runID: string, limit = 100, offset = 0) {
    const query = new URLSearchParams({
      limit: String(limit),
      offset: String(offset)
    });
    return this.request<SyncSkippedRow[]>(`/sync/runs/${encodeURIComponent(runID)}/skipped?${query.toString()}`);
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
      ? await this.readJSONPayload(response)
      : null;

    if (!response.ok) {
      throw normalizeError(payload, response.status);
    }

    return payload as T;
  }

  private async readJSONPayload(response: Response) {
    const payload: unknown = await response.json();
    if (response.headers.get(payloadEncryptionHeader) !== payloadEncryptionAlgo) {
      return payload;
    }
    return decryptPayloadEnvelope(payload, this.getPayloadEncryptionKey().trim(), response.status);
  }
}

async function decryptPayloadEnvelope(payload: unknown, keyB64: string, httpStatus?: number) {
  if (!keyB64) {
    throw payloadEncryptionError(
      "暗号化された API レスポンスを復号できません。API 暗号化 key に AES-GCM key を入力してください。",
      httpStatus
    );
  }
  if (!globalThis.crypto?.subtle) {
    throw payloadEncryptionError("このブラウザーは AES-GCM 復号に対応していません。", httpStatus);
  }

  const envelope = payload as PayloadEnvelope;
  if (
    !envelope ||
    typeof envelope !== "object" ||
    envelope.enc !== payloadEncryptionAlgo ||
    typeof envelope.nonce !== "string" ||
    typeof envelope.ciphertext !== "string"
  ) {
    throw payloadEncryptionError("暗号化された API レスポンスの形式が不正です。", httpStatus);
  }

  try {
    const keyBytes = base64ToBytes(keyB64);
    if (keyBytes.length !== payloadEncryptionKeyBytes) {
      throw new Error("invalid key length");
    }
    const cryptoKey = await globalThis.crypto.subtle.importKey("raw", keyBytes, "AES-GCM", false, ["decrypt"]);
    const plaintext = await globalThis.crypto.subtle.decrypt(
      { name: "AES-GCM", iv: base64ToBytes(envelope.nonce) },
      cryptoKey,
      base64ToBytes(envelope.ciphertext)
    );
    return JSON.parse(new TextDecoder().decode(plaintext));
  } catch {
    throw payloadEncryptionError("API レスポンスの復号に失敗しました。AES-GCM key を確認してください。", httpStatus);
  }
}

function payloadEncryptionError(message: string, httpStatus?: number): ApiError {
  return {
    status: "invalid_request",
    message,
    httpStatus
  };
}

function base64ToBytes(value: string) {
  const binary = atob(value);
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
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
