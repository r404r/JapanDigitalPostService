import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent } from "react";
import { ApiClient } from "./api/client";
import type {
  Address,
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
      <form className="panel" onSubmit={create}>
        <div className="section-heading">
          <h2>Token 管理</h2>
          <p>明文 token は発行直後に一度だけ表示されます。</p>
        </div>
        <div className="input-grid compact">
          <TextInput label="名前" value={form.name} onChange={(name) => setForm({ ...form, name })} />
          <label>
            <span>scope</span>
            <select value={form.scope} onChange={(event) => setForm({ ...form, scope: event.target.value as "read" | "admin" })}>
              <option value="read">read</option>
              <option value="admin">admin</option>
            </select>
          </label>
        </div>
        <div className="actions">
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
