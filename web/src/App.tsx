import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { ApiClient } from "./api/client";
import type {
  Address,
  ApiError,
  CreatedToken,
  SearchResult,
  SyncRun,
  SyncStatus,
  TokenInfo
} from "./api/client";

type Page = "search" | "sync" | "tokens";

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
        <button className={page === "sync" ? "active" : ""} onClick={() => setPage("sync")}>
          同期
        </button>
        <button className={page === "tokens" ? "active" : ""} onClick={() => setPage("tokens")}>
          Tokens
        </button>
      </nav>

      <main>
        {page === "search" && <SearchPage api={api} hasToken={Boolean(token.trim())} />}
        {page === "sync" && <SyncPage api={api} hasToken={Boolean(token.trim())} />}
        {page === "tokens" && <TokenPage api={api} hasToken={Boolean(token.trim())} />}
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

  const addresses = result?.items ?? result?.addresses ?? [];
  const returned = result?.returned_count ?? result?.returned ?? addresses.length;
  const addressCount = result?.address_count ?? addresses.length;

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
            <Metric label="returned_count" value={returned} />
            <Metric label="address_count" value={addressCount} />
          </div>
          <SearchStatus result={result} />
          {addresses.length === 0 ? <EmptyState text="該当する住所はありません。" /> : <AddressTable addresses={addresses} />}
        </section>
      )}
    </section>
  );
}

function SyncPage({ api, hasToken }: { api: ApiClient; hasToken: boolean }) {
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [runs, setRuns] = useState<SyncRun[]>([]);
  const [error, setError] = useState<ApiError | null>(null);
  const [loading, setLoading] = useState(false);
  const [triggerType, setTriggerType] = useState<"full" | "diff">("diff");

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const [nextStatus, nextRuns] = await Promise.all([api.getSyncStatus(), api.listSyncRuns()]);
      setStatus(nextStatus);
      setRuns(nextRuns);
    } catch (caught) {
      setError(caught as ApiError);
    } finally {
      setLoading(false);
    }
  };

  const trigger = async () => {
    setLoading(true);
    setError(null);
    try {
      await api.triggerSync(triggerType);
      await load();
    } catch (caught) {
      setError(caught as ApiError);
      setLoading(false);
    }
  };

  return (
    <section className="workspace">
      <section className="panel">
        <div className="section-heading">
          <h2>同期状態</h2>
          <p>現在のデータ量、直近成功同期、履歴を確認します。</p>
        </div>
        <div className="actions">
          <button type="button" onClick={load} disabled={loading || !hasToken}>
            {loading ? "読込中" : "再読込"}
          </button>
          <select value={triggerType} onChange={(event) => setTriggerType(event.target.value as "full" | "diff")}>
            <option value="diff">diff</option>
            <option value="full">full</option>
          </select>
          <button type="button" onClick={trigger} disabled={loading || !hasToken}>
            手動同期
          </button>
          {!hasToken && <span className="hint">admin token が必要です。</span>}
        </div>
      </section>

      {error && <StatusNotice error={error} />}
      {status && (
        <section className="panel">
          <div className="metric-row">
            <Metric label="address_count" value={status.address_count ?? status.total_addresses ?? 0} />
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

function TokenPage({ api, hasToken }: { api: ApiClient; hasToken: boolean }) {
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
    <section className="workspace">
      <form className="panel" onSubmit={create}>
        <div className="section-heading">
          <h2>Token 発行</h2>
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
              <td>{run.processed_count ?? run.rows_total ?? countRows(run)}</td>
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
