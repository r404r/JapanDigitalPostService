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
  SyncStatus,
  SyncType,
  TokenInfo
} from "./api/client";

type Page = "search" | "admin";

export default function App() {
  const [page, setPage] = useState<Page>("search");
  const [token, setToken] = useState(() => sessionStorage.getItem("apiToken") ?? "");
  const api = useMemo(() => new ApiClient(() => token), [token]);

  const updateToken = (value: string) => {
    setToken(value);
    if (value.trim()) {
      sessionStorage.setItem("apiToken", value);
    } else {
      sessionStorage.removeItem("apiToken");
    }
  };

  return (
    <div className="app-shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">React sample</p>
          <h1>JapanDigitalPostService</h1>
        </div>
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
    setError(null);
    setLoading(false);
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
        {runs.length === 0 ? <EmptyState text="同期履歴はまだありません。" /> : <SyncRunsTable runs={runs} />}
      </section>
    </section>
  );
}

type SettingsForm = {
  download_max_retry: string;
  scrape_full_url: string;
};

function SettingsPanel({ api, hasToken }: { api: ApiClient; hasToken: boolean }) {
  const [settings, setSettings] = useState<AdminSettings | null>(null);
  const [form, setForm] = useState<SettingsForm>({ download_max_retry: "", scrape_full_url: "" });
  const [error, setError] = useState<ApiError | null>(null);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);

  const applySettings = (nextSettings: AdminSettings) => {
    setSettings(nextSettings);
    setForm({
      download_max_retry: String(nextSettings.download_max_retry.value),
      scrape_full_url: nextSettings.scrape_full_url.value
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
      setForm({ download_max_retry: "", scrape_full_url: "" });
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
          scrape_full_url: form.scrape_full_url.trim()
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
          reset_to_default: ["download_max_retry", "scrape_full_url"]
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
        <p>全量取得 URL とダウンロードのリトライ回数を変更できます。</p>
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

function SyncRunsTable({ runs }: { runs: SyncRun[] }) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>type</th>
            <th>status</th>
            <th>time</th>
            <th>processed</th>
            <th>error</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((run) => (
            <tr key={run.id}>
              <td>{run.type}</td>
              <td>{run.status}</td>
              <td>{formatDate(run.started_at)} - {formatDate(run.finished_at)}</td>
              <td>{run.rows_total ?? countRows(run)}</td>
              <td>{run.error_message ?? "-"}</td>
            </tr>
          ))}
        </tbody>
      </table>
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
  return (run.rows_added ?? 0) + (run.rows_updated ?? 0) + (run.rows_deleted ?? 0);
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
