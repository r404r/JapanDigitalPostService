import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { DragEvent, FormEvent } from "react";
import { ApiClient } from "./api/client";
import type {
  Address,
  AdminSettings,
  ApiError,
  CreatedToken,
  SearchResult,
  SyncRun,
  SyncSkippedRow,
  SyncStatus,
  SyncType,
  TokenInfo
} from "./api/client";

type Page = "search" | "admin";

export default function App() {
  const [page, setPage] = useState<Page>("search");
  const [token, setToken] = useState(() => sessionStorage.getItem("apiToken") ?? "");
  const [payloadEncryptionKey, setPayloadEncryptionKey] = useState(() => sessionStorage.getItem("payloadEncryptionKey") ?? "");
  const api = useMemo(
    () => new ApiClient(() => token, "/v1", () => payloadEncryptionKey),
    [token, payloadEncryptionKey]
  );

  const updateToken = (value: string) => {
    setToken(value);
    if (value.trim()) {
      sessionStorage.setItem("apiToken", value);
    } else {
      sessionStorage.removeItem("apiToken");
    }
  };

  const updatePayloadEncryptionKey = (value: string) => {
    setPayloadEncryptionKey(value);
    if (value.trim()) {
      sessionStorage.setItem("payloadEncryptionKey", value);
    } else {
      sessionStorage.removeItem("payloadEncryptionKey");
    }
  };

  return (
    <div className="app-shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">React sample</p>
          <h1>JapanDigitalPostService</h1>
        </div>
        <div className="credential-fields">
          <label className="token-field">
            <span>Bearer token</span>
            <input
              value={token}
              onChange={(event) => updateToken(event.target.value)}
              placeholder="admin/read token"
              type="password"
              autoComplete="off"
            />
          </label>
          <label className="token-field">
            <span>API 暗号化 key</span>
            <input
              value={payloadEncryptionKey}
              onChange={(event) => updatePayloadEncryptionKey(event.target.value)}
              placeholder="base64 32B key"
              type="password"
              autoComplete="off"
            />
          </label>
        </div>
      </header>

      <nav className="tabs" aria-label="Sample pages">
        <button className={page === "search" ? "active" : ""} onClick={() => setPage("search")}>
          検索
        </button>
        <button className={page === "admin" ? "active" : ""} onClick={() => setPage("admin")}>
          管理
        </button>
      </nav>

      <main>
        {page === "search" && <SearchPage api={api} hasToken={Boolean(token.trim())} />}
        {page === "admin" && <AdminPage api={api} hasToken={Boolean(token.trim())} />}
      </main>
    </div>
  );
}

function SearchPage({ api, hasToken }: { api: ApiClient; hasToken: boolean }) {
  const [form, setForm] = useState({ zipcode: "", prefecture: "", city: "", q: "" });
  const [result, setResult] = useState<SearchResult | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    setResult(null);
    setLoading(true);
    try {
      setResult(await api.searchAddresses(form));
    } catch (caught) {
      setError(caught as ApiError);
    } finally {
      setLoading(false);
    }
  };

  const addresses = result?.items ?? [];

  return (
    <section className="workspace">
      <form className="panel search-form" onSubmit={submit}>
        <div className="section-heading">
          <h2>住所検索</h2>
          <p>郵便番号、都道府県、市区町村、キーワードを組み合わせて検索します。</p>
        </div>
        <div className="input-grid">
          <TextInput label="郵便番号" value={form.zipcode} onChange={(zipcode) => setForm({ ...form, zipcode })} />
          <TextInput
            label="都道府県"
            value={form.prefecture}
            onChange={(prefecture) => setForm({ ...form, prefecture })}
          />
          <TextInput label="市区町村" value={form.city} onChange={(city) => setForm({ ...form, city })} />
          <TextInput label="キーワード" value={form.q} onChange={(q) => setForm({ ...form, q })} />
        </div>
        <div className="actions">
          <button type="submit" disabled={loading || !hasToken}>
            {loading ? "検索中" : "検索実行"}
          </button>
          {!hasToken && <span className="hint">Bearer token を入力してください。</span>}
        </div>
      </form>

      {error && <StatusNotice error={error} />}
      {result && (
        <section className="panel">
          <div className="metric-row">
            <Metric label="total_count" value={result.total_count} />
            <Metric label="returned" value={result.returned_count} />
            <Metric label="items.length" value={addresses.length} />
          </div>
          <SearchStatus result={result} />
          {addresses.length === 0 ? <EmptyState text="該当する住所はありません。" /> : <AddressTable addresses={addresses} />}
        </section>
      )}
    </section>
  );
}

function AdminPage({ api, hasToken }: { api: ApiClient; hasToken: boolean }) {
  return (
    <section className="workspace admin-workspace">
      <SyncPanel api={api} hasToken={hasToken} />
      <TokenPanel api={api} hasToken={hasToken} />
    </section>
  );
}

function SyncPanel({ api, hasToken }: { api: ApiClient; hasToken: boolean }) {
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [runs, setRuns] = useState<SyncRun[]>([]);
  const [triggered, setTriggered] = useState<SyncRun | null>(null);
  const [selectedSkippedRun, setSelectedSkippedRun] = useState<SyncRun | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(false);
  const [triggerType, setTriggerType] = useState<SyncType>("auto");
  const loadGeneration = useRef(0);

  const load = useCallback(async () => {
    const generation = ++loadGeneration.current;
    setLoading(true);
    setError(null);
    try {
      const [nextStatus, nextRuns] = await Promise.all([api.getSyncStatus(), api.listSyncRuns()]);
      if (loadGeneration.current !== generation) {
        return;
      }
      setStatus(nextStatus);
      setRuns(nextRuns);
      setSelectedSkippedRun((currentRun) => nextRuns.find((run) => run.id === currentRun?.id) ?? null);
    } catch (caught) {
      if (loadGeneration.current === generation) {
        setError(caught as ApiError);
      }
    } finally {
      if (loadGeneration.current === generation) {
        setLoading(false);
      }
    }
  }, [api]);

  const reset = useCallback(() => {
    loadGeneration.current += 1;
    setStatus(null);
    setRuns([]);
    setTriggered(null);
    setSelectedSkippedRun(null);
    setError(null);
    setLoading(false);
  }, []);

  const closeSkippedRows = useCallback(() => {
    setSelectedSkippedRun(null);
  }, []);

  useEffect(() => {
    if (!hasToken) {
      reset();
      return;
    }
    void load();
  }, [hasToken, load, reset]);

  const trigger = async () => {
    setLoading(true);
    setError(null);
    setTriggered(null);
    try {
      const runningRun = await api.triggerSync(triggerType);
      setTriggered(runningRun);
      setRuns((currentRuns) => [runningRun, ...currentRuns.filter((run) => run.id !== runningRun.id)]);
      setStatus((currentStatus) => currentStatus ? { ...currentStatus, running: true } : currentStatus);
      await load();
    } catch (caught) {
      setError(caught as ApiError);
      setLoading(false);
    }
  };

  return (
    <section className="admin-section">
      <section className="panel sync-control-panel">
        <div className="section-heading">
          <h2>同期管理</h2>
          <p>状態確認と手動同期を分けて操作できます。</p>
        </div>
        <div className="sync-controls">
          <section className="sync-control" aria-labelledby="sync-refresh-title">
            <div>
              <h3 id="sync-refresh-title">状態を更新</h3>
              <p>現在の状態と最新 100 件の履歴だけを再取得します。</p>
            </div>
            <button className="secondary-button" type="button" onClick={load} disabled={loading || !hasToken}>
              {loading ? "読込中" : "状態を再読込"}
            </button>
          </section>

          <section className="sync-control sync-control-primary" aria-labelledby="sync-trigger-title">
            <div>
              <h3 id="sync-trigger-title">同期を開始</h3>
              <p>方式を選択してから同期を実行します。</p>
            </div>
            <div className="sync-mode-row">
              <label className="sync-mode-field">
                <span>同期方式</span>
                <select
                  value={triggerType}
                  onChange={(event) => setTriggerType(event.target.value as SyncType)}
                  disabled={loading || !hasToken}
                >
                  <option value="auto">auto（自動判定）</option>
                  <option value="diff">diff（差分）</option>
                  <option value="full">full（全量）</option>
                </select>
              </label>
              <button className="sync-submit-button" type="button" onClick={trigger} disabled={loading || !hasToken}>
                選択した方式で同期実行
              </button>
            </div>
            <p className="field-note">下のボタンは選択した方式で同期を開始します。</p>
            <p className="field-note">auto はデータ件数に応じて full/diff を自動判定します。</p>
          </section>
        </div>
        <div className="actions">
          {!hasToken && <span className="hint">admin token が必要です。</span>}
        </div>
      </section>

      {triggered && (
        <div className="notice info" role="status">
          <strong>{triggered.type}</strong>
          <span>{triggered.status} として受け付けました。状態と履歴を更新しています。</span>
        </div>
      )}
      {error && <StatusNotice error={error} />}
      <SettingsPanel api={api} hasToken={hasToken} />
      <UploadPanel api={api} hasToken={hasToken} onUploaded={load} />
      {status && (
        <section className="panel">
          <div className="metric-row">
            <Metric label="total_addresses" value={status.total_addresses} />
            <Metric label="running" value={status.running ? "yes" : "no"} />
            <Metric label="last_type" value={status.last_type ?? "-"} />
            <Metric label="last_success_at" value={formatDate(status.last_success_at)} />
          </div>
        </section>
      )}
      <section className="panel">
        <h3>同期履歴</h3>
        {runs.length === 0 ? (
          <EmptyState text="同期履歴はまだありません。" />
        ) : (
          <SyncRunsTable
            runs={runs}
            selectedRunID={selectedSkippedRun?.id ?? null}
            onShowSkippedRows={setSelectedSkippedRun}
          />
        )}
      </section>
      <SkippedRowsModal
        api={api}
        run={selectedSkippedRun}
        hasToken={hasToken}
        onClose={closeSkippedRows}
      />
    </section>
  );
}

type SettingsForm = {
  download_max_retry: string;
  scrape_full_url: string;
  town_skip_regex: string;
};

function SettingsPanel({ api, hasToken }: { api: ApiClient; hasToken: boolean }) {
  const [settings, setSettings] = useState<AdminSettings | null>(null);
  const [form, setForm] = useState<SettingsForm>({
    download_max_retry: "",
    scrape_full_url: "",
    town_skip_regex: ""
  });
  const [error, setError] = useState<ApiError | null>(null);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);

  const applySettings = (nextSettings: AdminSettings) => {
    setSettings(nextSettings);
    setForm({
      download_max_retry: String(nextSettings.download_max_retry.value),
      scrape_full_url: nextSettings.scrape_full_url.value,
      town_skip_regex: nextSettings.town_skip_regex.value
    });
  };

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    setMessage("");
    try {
      applySettings(await api.getAdminSettings());
    } catch (caught) {
      setError(caught as ApiError);
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => {
    if (!hasToken) {
      setSettings(null);
      setForm({ download_max_retry: "", scrape_full_url: "", town_skip_regex: "" });
      setError(null);
      setMessage("");
      return;
    }
    void load();
  }, [hasToken, load]);

  const validate = () => {
    const retry = Number(form.download_max_retry);
    if (!Number.isInteger(retry) || retry < 0 || retry > 10) {
      return "リトライ回数は 0 以上 10 以下の整数で指定してください。";
    }
    try {
      const parsed = new URL(form.scrape_full_url);
      if (parsed.protocol !== "https:") {
        return "URL は https で指定してください。";
      }
      if (parsed.username || parsed.password || !["post.japanpost.jp", "www.post.japanpost.jp"].includes(parsed.hostname)) {
        return "URL のドメインは日本郵便の公式サイト（post.japanpost.jp）のみ許可されています。";
      }
    } catch {
      return "URL の形式を確認してください。";
    }
    return "";
  };

  const save = async (event: FormEvent) => {
    event.preventDefault();
    const validationError = validate();
    setError(null);
    setMessage("");
    if (validationError) {
      setError({ status: "invalid_request", message: validationError });
      return;
    }
    setLoading(true);
    try {
      applySettings(
        await api.updateAdminSettings({
          download_max_retry: Number(form.download_max_retry),
          scrape_full_url: form.scrape_full_url.trim(),
          town_skip_regex: form.town_skip_regex.trim()
        })
      );
      setMessage("設定を保存しました。");
    } catch (caught) {
      setError(caught as ApiError);
    } finally {
      setLoading(false);
    }
  };

  const resetToDefault = async () => {
    setLoading(true);
    setError(null);
    setMessage("");
    try {
      applySettings(
        await api.updateAdminSettings({
          reset_to_default: ["download_max_retry", "scrape_full_url", "town_skip_regex"]
        })
      );
      setMessage("既定値に戻しました。");
    } catch (caught) {
      setError(caught as ApiError);
    } finally {
      setLoading(false);
    }
  };

  return (
    <form className="panel settings-form" onSubmit={save}>
      <div className="section-heading">
        <h2>取得設定</h2>
        <p>取得 URL、リトライ回数、町域名フィルターを変更できます。</p>
      </div>
      <div className="input-grid settings-grid">
        <label>
          <span>リトライ回数</span>
          <input
            aria-label="リトライ回数"
            inputMode="numeric"
            value={form.download_max_retry}
            onChange={(event) => setForm({ ...form, download_max_retry: event.target.value })}
            disabled={!hasToken || loading}
          />
          <small>0 から 10 まで。現在: {settings?.download_max_retry.overridden ? "変更済み" : "既定値"}</small>
        </label>
        <label>
          <span>全量取得 URL</span>
          <input
            aria-label="全量取得 URL"
            value={form.scrape_full_url}
            onChange={(event) => setForm({ ...form, scrape_full_url: event.target.value })}
            disabled={!hasToken || loading}
          />
          <small>https://post.japanpost.jp または https://www.post.japanpost.jp のみ許可。</small>
        </label>
        <label>
          <span>町域名フィルター</span>
          <input
            aria-label="町域名フィルター"
            value={form.town_skip_regex}
            placeholder="^(?:以下に掲載がない場合)$"
            onChange={(event) => setForm({ ...form, town_skip_regex: event.target.value })}
            disabled={!hasToken || loading}
          />
          <small>
            空欄で無効。Go 正規表現として保存時に検証され、次回の同期またはアップロード取り込みで適用されます。
            現在: {settings?.town_skip_regex.overridden ? "変更済み" : "既定値"}
          </small>
        </label>
      </div>
      <div className="actions">
        <button type="submit" disabled={!hasToken || loading}>
          保存
        </button>
        <button className="secondary-button" type="button" onClick={resetToDefault} disabled={!hasToken || loading}>
          既定値に戻す
        </button>
        <button className="secondary-button" type="button" onClick={load} disabled={!hasToken || loading}>
          設定を再読込
        </button>
        {!hasToken && <span className="hint">admin token が必要です。</span>}
      </div>
      {message && <div className="notice success" role="status">{message}</div>}
      {error && <StatusNotice error={error} />}
    </form>
  );
}

function UploadPanel({ api, hasToken, onUploaded }: { api: ApiClient; hasToken: boolean; onUploaded: () => Promise<void> }) {
  const [file, setFile] = useState<File | null>(null);
  const [dragging, setDragging] = useState(false);
  const [result, setResult] = useState<SyncRun | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(false);

  const selectFile = (nextFile?: File) => {
    setResult(null);
    setError(null);
    if (!nextFile) {
      setFile(null);
      return;
    }
    if (!/\.(csv|zip)$/i.test(nextFile.name)) {
      setFile(null);
      setError({ status: "unsupported_file", message: "zip または csv ファイルを選択してください。" });
      return;
    }
    setFile(nextFile);
  };

  const drop = (event: DragEvent<HTMLLabelElement>) => {
    event.preventDefault();
    setDragging(false);
    selectFile(event.dataTransfer.files[0]);
  };

  const upload = async () => {
    if (!file) {
      setError({ status: "invalid_request", message: "アップロードするファイルを選択してください。" });
      return;
    }
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      const nextRun = await api.uploadSync(file);
      setResult(nextRun);
      await onUploaded();
    } catch (caught) {
      setError(caught as ApiError);
    } finally {
      setLoading(false);
    }
  };

  return (
    <section className="panel upload-panel">
      <div className="section-heading">
        <h2>ファイルアップロード</h2>
        <p>zip または UTF-8 csv をアップロードして全量同期として取り込みます。</p>
      </div>
      <label
        className={`dropzone${dragging ? " dragging" : ""}`}
        onDragOver={(event) => {
          event.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={drop}
      >
        <span>{file ? file.name : "zip/csv をドラッグ、またはクリックして選択"}</span>
        <input
          aria-label="zip/csv をドラッグ、またはクリックして選択"
          type="file"
          accept=".zip,.csv,application/zip,text/csv"
          disabled={!hasToken || loading}
          onChange={(event) => selectFile(event.target.files?.[0])}
        />
      </label>
      <div className="actions">
        <button type="button" onClick={upload} disabled={!hasToken || loading || !file}>
          {loading ? "アップロード中" : "アップロード実行"}
        </button>
        {!hasToken && <span className="hint">admin token が必要です。</span>}
      </div>
      {result && (
        <div className={`notice ${result.status === "success" ? "success" : "warning"}`} role="status">
          <strong>{result.status === "success" ? "取り込みが完了しました。" : "取り込み結果を確認してください。"}</strong>
          <span>追加 {result.rows_added ?? 0} / 更新 {result.rows_updated ?? 0} / 削除 {result.rows_deleted ?? 0} / 合計 {result.rows_total ?? countRows(result)}</span>
          {result.error_message && <span>{result.error_message}</span>}
        </div>
      )}
      {error && <StatusNotice error={error} />}
    </section>
  );
}

function TokenPanel({ api, hasToken }: { api: ApiClient; hasToken: boolean }) {
  const [tokens, setTokens] = useState<TokenInfo[]>([]);
  const [created, setCreated] = useState<CreatedToken | null>(null);
  const [form, setForm] = useState({ name: "", scope: "read" as "read" | "admin" });
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(false);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      setTokens(await api.listTokens());
    } catch (caught) {
      setError(caught as ApiError);
    } finally {
      setLoading(false);
    }
  };

  const create = async (event: FormEvent) => {
    event.preventDefault();
    setLoading(true);
    setError(null);
    setCreated(null);
    try {
      const nextToken = await api.createToken(form);
      setCreated(nextToken);
      setForm({ ...form, name: "" });
      await load();
    } catch (caught) {
      setError(caught as ApiError);
      setLoading(false);
    }
  };

  const revoke = async (id: string) => {
    setLoading(true);
    setError(null);
    try {
      await api.revokeToken(id);
      await load();
    } catch (caught) {
      setError(caught as ApiError);
      setLoading(false);
    }
  };

  return (
    <section className="admin-section">
      <form className="panel token-form" onSubmit={create}>
        <div className="section-heading">
          <h2>Token 管理</h2>
          <p>明文 token は発行直後に一度だけ表示されます。</p>
        </div>
        <div className="input-grid compact token-fields">
          <TextInput label="名前" value={form.name} onChange={(name) => setForm({ ...form, name })} />
          <label>
            <span>scope</span>
            <select value={form.scope} onChange={(event) => setForm({ ...form, scope: event.target.value as "read" | "admin" })}>
              <option value="read">read</option>
              <option value="admin">admin</option>
            </select>
          </label>
        </div>
        <div className="actions token-actions" role="group" aria-label="Token 操作">
          <button type="submit" disabled={loading || !hasToken || !form.name.trim()}>
            発行
          </button>
          <button type="button" onClick={load} disabled={loading || !hasToken}>
            一覧更新
          </button>
          {!hasToken && <span className="hint">admin token を入力してください。</span>}
        </div>
      </form>

      {created && (
        <section className="panel token-once" role="status">
          <h3>保存してください</h3>
          <p>この明文 token はこの画面で一度だけ確認できます。閉じる前に安全な場所へ保存してください。</p>
          <code>{created.token}</code>
          <button type="button" onClick={() => setCreated(null)}>
            非表示
          </button>
        </section>
      )}
      {error && <StatusNotice error={error} />}
      <section className="panel">
        <h3>Token 一覧</h3>
        {tokens.length === 0 ? <EmptyState text="表示できる token はありません。" /> : <TokenTable tokens={tokens} onRevoke={revoke} loading={loading} />}
      </section>
    </section>
  );
}

function TextInput({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label>
      <span>{label}</span>
      <input value={value} onChange={(event) => onChange(event.target.value)} />
    </label>
  );
}

function Metric({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function SearchStatus({ result }: { result: SearchResult }) {
  if (result.status === "too_many_results") {
    return <div className="notice warning">結果が多すぎます。都道府県や市区町村で条件を絞ってください。</div>;
  }
  if (result.status === "timeout") {
    return <div className="notice warning">検索がタイムアウトしました。条件を絞って再試行してください。</div>;
  }
  if (result.truncated) {
    return <div className="notice info">表示上限により結果を一部だけ表示しています。</div>;
  }
  return null;
}

function StatusNotice({ error }: { error: ApiError }) {
  return (
    <div className="notice error" role="alert">
      <strong>{error.status}</strong>
      <span>{error.message}</span>
    </div>
  );
}

function AddressTable({ addresses }: { addresses: Address[] }) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>郵便番号</th>
            <th>都道府県</th>
            <th>市区町村</th>
            <th>町域</th>
            <th>カナ</th>
          </tr>
        </thead>
        <tbody>
          {addresses.map((address, index) => (
            <tr key={`${address.zipcode}-${address.prefecture}-${address.city}-${address.town}-${index}`}>
              <td>{address.zipcode}</td>
              <td>{address.prefecture}</td>
              <td>{address.city}</td>
              <td>{address.town}</td>
              <td>{[address.prefecture_kana, address.city_kana, address.town_kana].filter(Boolean).join(" ")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function SyncRunsTable({
  runs,
  selectedRunID,
  onShowSkippedRows
}: {
  runs: SyncRun[];
  selectedRunID: string | null;
  onShowSkippedRows: (run: SyncRun) => void;
}) {
  const [pageIndex, setPageIndex] = useState(0);
  const totalPages = Math.max(1, Math.ceil(runs.length / syncRunsPageSize));
  const currentPageRuns = runs.slice(pageIndex * syncRunsPageSize, (pageIndex + 1) * syncRunsPageSize);
  const canGoPrevious = pageIndex > 0;
  const canGoNext = pageIndex + 1 < totalPages;

  useEffect(() => {
    setPageIndex((currentPageIndex) => Math.min(currentPageIndex, totalPages - 1));
  }, [totalPages]);

  return (
    <div className="sync-runs">
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>type</th>
              <th>status</th>
              <th>time</th>
              <th>processed</th>
              <th>skipped</th>
              <th>error</th>
            </tr>
          </thead>
          <tbody>
            {currentPageRuns.map((run) => {
              const skipped = skippedRows(run);
              return (
                <tr key={run.id}>
                  <td>{run.type}</td>
                  <td>{run.status}</td>
                  <td>{formatDate(run.started_at)} - {formatDate(run.finished_at)}</td>
                  <td>{run.rows_total ?? countRows(run)}</td>
                  <td>
                    {skipped > 0 ? (
                      <span className="skipped-cell">
                        <span className="skipped-count">{skipped}</span>
                        <button
                          className="table-action"
                          type="button"
                          aria-pressed={run.id === selectedRunID}
                          onClick={() => onShowSkippedRows(run)}
                        >
                          照会
                        </button>
                      </span>
                    ) : (
                      "-"
                    )}
                  </td>
                  <td>{run.error_message ?? "-"}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {totalPages > 1 && (
        <div className="table-pagination" aria-label="同期履歴ページ操作">
          <span>
            ページ {pageIndex + 1} / {totalPages}
          </span>
          <div className="pagination-buttons">
            <button
              className="secondary-button"
              type="button"
              onClick={() => setPageIndex((currentPageIndex) => Math.max(0, currentPageIndex - 1))}
              disabled={!canGoPrevious}
            >
              前へ
            </button>
            <button
              className="secondary-button"
              type="button"
              onClick={() => setPageIndex((currentPageIndex) => Math.min(totalPages - 1, currentPageIndex + 1))}
              disabled={!canGoNext}
            >
              次へ
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

const syncRunsPageSize = 6;
const skippedRowsPageSize = 100;

function SkippedRowsModal({
  api,
  run,
  hasToken,
  onClose
}: {
  api: ApiClient;
  run: SyncRun | null;
  hasToken: boolean;
  onClose: () => void;
}) {
  const [rows, setRows] = useState<SyncSkippedRow[]>([]);
  const [pageIndex, setPageIndex] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  const [expandedRawIDs, setExpandedRawIDs] = useState<Set<number>>(() => new Set());
  const dialogRef = useRef<HTMLElement | null>(null);
  const closeButtonRef = useRef<HTMLButtonElement | null>(null);
  const pageLoadGeneration = useRef(0);
  const totalSkipped = run ? skippedRows(run) : 0;
  const totalPages = Math.max(1, Math.ceil(totalSkipped / skippedRowsPageSize));

  const loadPage = useCallback(
    async (nextPageIndex: number) => {
      if (!run || !hasToken) {
        return;
      }
      const generation = ++pageLoadGeneration.current;
      setLoading(true);
      setError(null);
      setExpandedRawIDs(new Set());
      try {
        const nextOffset = nextPageIndex * skippedRowsPageSize;
        const nextRows = await api.listSyncSkippedRows(run.id, skippedRowsPageSize, nextOffset);
        if (pageLoadGeneration.current !== generation) {
          return;
        }
        setRows(nextRows);
        setPageIndex(nextPageIndex);
      } catch (caught) {
        if (pageLoadGeneration.current === generation) {
          setError(caught as ApiError);
        }
      } finally {
        if (pageLoadGeneration.current === generation) {
          setLoading(false);
        }
      }
    },
    [api, run, hasToken]
  );

  useEffect(() => {
    setRows([]);
    setPageIndex(0);
    setError(null);
    setExpandedRawIDs(new Set());
    if (!run || !hasToken) {
      pageLoadGeneration.current += 1;
      setLoading(false);
      return;
    }
    void loadPage(0);
  }, [run, hasToken, loadPage]);

  useEffect(() => {
    if (!run) {
      return;
    }
    const previousOverflow = document.body.style.overflow;
    const previouslyFocusedElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.body.style.overflow = "hidden";
    closeButtonRef.current?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
        return;
      }
      if (event.key !== "Tab") {
        return;
      }
      const focusableElements = dialogRef.current?.querySelectorAll<HTMLElement>(
        'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
      );
      if (!focusableElements || focusableElements.length === 0) {
        return;
      }
      const firstElement = focusableElements[0];
      const lastElement = focusableElements[focusableElements.length - 1];
      if (event.shiftKey && document.activeElement === firstElement) {
        event.preventDefault();
        lastElement.focus();
      } else if (!event.shiftKey && document.activeElement === lastElement) {
        event.preventDefault();
        firstElement.focus();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener("keydown", handleKeyDown);
      previouslyFocusedElement?.focus();
    };
  }, [run, onClose]);

  const toggleRaw = (rowID: number) => {
    setExpandedRawIDs((currentIDs) => {
      const nextIDs = new Set(currentIDs);
      if (nextIDs.has(rowID)) {
        nextIDs.delete(rowID);
      } else {
        nextIDs.add(rowID);
      }
      return nextIDs;
    });
  };

  if (!run) {
    return null;
  }

  const canGoPrevious = pageIndex > 0;
  const canGoNext = pageIndex + 1 < totalPages;

  return (
    <div
      className="modal-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <section
        ref={dialogRef}
        className="modal-card skipped-rows-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="skipped-rows-title"
      >
        <div className="modal-header skipped-rows-heading">
          <div>
            <h3 id="skipped-rows-title">除外行明細</h3>
            <p>
              run {run.id} / 除外 {totalSkipped} 件。同期履歴から読み込んだ明細を表示します。
            </p>
          </div>
          <button ref={closeButtonRef} className="secondary-button" type="button" onClick={onClose}>
            閉じる
          </button>
        </div>
        <div className="modal-body skipped-rows-modal-body">
          {error && <StatusNotice error={error} />}
          {loading && rows.length === 0 && <p className="empty">除外行を読み込んでいます。</p>}
          {!loading && rows.length === 0 && !error && <EmptyState text="除外行はありません。" />}
          {rows.length > 0 && (
            <div className="table-wrap">
              <table className="skipped-rows-table">
                <thead>
                  <tr>
                    <th>source</th>
                    <th>line</th>
                    <th>zipcode</th>
                    <th>prefecture / city / town</th>
                    <th>town_kana</th>
                    <th>pattern</th>
                    <th>raw</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr key={row.id}>
                      <td>{row.source_type}</td>
                      <td>{row.line_number}</td>
                      <td>{row.zipcode ?? "-"}</td>
                      <td>{[row.prefecture, row.city, row.town].filter(Boolean).join(" / ") || "-"}</td>
                      <td>{row.town_kana ?? "-"}</td>
                      <td>{row.pattern ?? "-"}</td>
                      <td>
                        <RawRecordCell
                          row={row}
                          expanded={expandedRawIDs.has(row.id)}
                          onToggle={() => toggleRaw(row.id)}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
        <div className="modal-footer skipped-rows-pagination" aria-label="除外行ページ操作">
          <span>
            ページ {pageIndex + 1} / {totalPages}
          </span>
          <div className="pagination-buttons">
            <button
              className="secondary-button"
              type="button"
              onClick={() => loadPage(pageIndex - 1)}
              disabled={!canGoPrevious || loading}
            >
              前へ
            </button>
            <button
              className="secondary-button"
              type="button"
              onClick={() => loadPage(pageIndex + 1)}
              disabled={!canGoNext || loading}
            >
              次へ
            </button>
          </div>
        </div>
      </section>
    </div>
  );
}

function RawRecordCell({
  row,
  expanded,
  onToggle
}: {
  row: SyncSkippedRow;
  expanded: boolean;
  onToggle: () => void;
}) {
  if (!row.raw_record_json) {
    return <span>-</span>;
  }
  return (
    <div className={`skipped-row-raw${expanded ? " expanded" : ""}`}>
      <code>{expanded ? row.raw_record_json : truncateText(row.raw_record_json, 36)}</code>
      <button className="inline-button" type="button" onClick={onToggle}>
        {expanded ? "raw を隠す" : "raw を表示"}
      </button>
    </div>
  );
}

function TokenTable({ tokens, onRevoke, loading }: { tokens: TokenInfo[]; onRevoke: (id: string) => void; loading: boolean }) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>name</th>
            <th>prefix</th>
            <th>scope</th>
            <th>created</th>
            <th>last used</th>
            <th>status</th>
            <th>action</th>
          </tr>
        </thead>
        <tbody>
          {tokens.map((token) => (
            <tr key={token.id}>
              <td>{token.name}</td>
              <td>{token.prefix}</td>
              <td>{token.scope}</td>
              <td>{formatDate(token.created_at)}</td>
              <td>{formatDate(token.last_used_at)}</td>
              <td>{token.revoked_at ? "revoked" : "active"}</td>
              <td>
                <button type="button" onClick={() => onRevoke(token.id)} disabled={loading || Boolean(token.revoked_at)}>
                  revoke
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EmptyState({ text }: { text: string }) {
  return <p className="empty">{text}</p>;
}

function countRows(run: SyncRun) {
  return (run.rows_added ?? 0) + (run.rows_updated ?? 0) + (run.rows_deleted ?? 0) + skippedRows(run);
}

function skippedRows(run: SyncRun) {
  return run.rows_skipped ?? 0;
}

function truncateText(value: string, maxLength: number) {
  if (value.length <= maxLength) {
    return value;
  }
  return `${value.slice(0, maxLength)}...`;
}

function formatDate(value?: string | null) {
  if (!value) {
    return "-";
  }
  return new Intl.DateTimeFormat("ja-JP", {
    dateStyle: "short",
    timeStyle: "medium"
  }).format(new Date(value));
}
