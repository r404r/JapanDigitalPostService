import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";

const jsonResponse = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "Content-Type": "application/json" },
    ...init
  });

const settingsBody = (
  overrides: Partial<Record<"download_max_retry" | "scrape_full_url" | "town_skip_regex", unknown>> = {}
) => ({
  download_max_retry: overrides.download_max_retry ?? { value: 3, default: 3, overridden: false },
  scrape_full_url:
    overrides.scrape_full_url ?? {
      value: "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip",
      default: "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip",
      overridden: false
    },
  town_skip_regex: overrides.town_skip_regex ?? { value: "", default: "", overridden: false }
});

const payloadEncKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=";
const wrongPayloadEncKey = "YWJjZGVmMDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODk=";
const fixedNonce = new Uint8Array([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11]);

const encryptedJsonResponse = async (body: unknown, init: ResponseInit = {}) => {
  const key = await crypto.subtle.importKey("raw", base64ToBytes(payloadEncKey), "AES-GCM", false, ["encrypt"]);
  const plaintext = new TextEncoder().encode(JSON.stringify(body));
  const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv: fixedNonce }, key, plaintext);

  return new Response(
    JSON.stringify({
      enc: "AES-256-GCM",
      kid: "k1",
      nonce: bytesToBase64(fixedNonce),
      ciphertext: bytesToBase64(new Uint8Array(ciphertext))
    }),
    {
      status: init.status ?? 200,
      headers: {
        "Content-Type": "application/json",
        "X-Payload-Encryption": "AES-256-GCM"
      },
      ...init
    }
  );
};

function base64ToBytes(value: string) {
  const binary = atob(value);
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}

function bytesToBase64(bytes: Uint8Array) {
  let binary = "";
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte);
  });
  return btoa(binary);
}

const skippedRow = (lineNumber: number, overrides: Record<string, unknown> = {}) => ({
  id: lineNumber,
  run_id: "run-skip",
  source_type: "full",
  line_number: lineNumber,
  zipcode: `100${String(lineNumber).padStart(4, "0")}`,
  jis_code: "13101",
  prefecture: "東京都",
  city: "千代田区",
  town: `除外町域${lineNumber}`,
  town_kana: `ジョガイ${lineNumber}`,
  reason: "town_regex",
  pattern: "(?i)町域",
  raw_record_json: JSON.stringify([
    `raw-row-${lineNumber}`,
    "東京都",
    "千代田区",
    `raw-tail-${lineNumber}`,
    "長い raw record の末尾確認用テキスト"
  ]),
  created_at: "2026-06-11T00:00:00Z",
  ...overrides
});

describe("App", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("searches addresses and shows result counts", async () => {
    sessionStorage.setItem("apiToken", "read-token");
    vi.mocked(fetch).mockResolvedValueOnce(
      jsonResponse({
        status: "ok",
        total_count: 2,
        returned_count: 1,
        truncated: false,
        items: [
          {
            zipcode: "1000001",
            prefecture: "東京都",
            city: "千代田区",
            town: "千代田",
            prefecture_kana: "トウキョウト",
            city_kana: "チヨダク",
            town_kana: "チヨダ"
          }
        ]
      })
    );

    render(<App />);
    await userEvent.type(screen.getByLabelText("郵便番号"), "1000001");
    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));

    expect(await screen.findByText("total_count")).toBeInTheDocument();
    expect(screen.getByText("returned")).toBeInTheDocument();
    expect(screen.getByText("items.length")).toBeInTheDocument();
    expect(screen.getByText("東京都")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/v1/addresses?zipcode=1000001"),
      expect.objectContaining({
        headers: expect.any(Headers)
      })
    );
  });

  it("decrypts encrypted API responses after configuring the AES-GCM key", async () => {
    sessionStorage.setItem("apiToken", "read-token");
    vi.mocked(fetch).mockResolvedValueOnce(
      await encryptedJsonResponse({
        status: "ok",
        total_count: 1,
        returned_count: 1,
        truncated: false,
        items: [
          {
            zipcode: "1000001",
            prefecture: "東京都",
            city: "千代田区",
            town: "千代田",
            prefecture_kana: "トウキョウト",
            city_kana: "チヨダク",
            town_kana: "チヨダ"
          }
        ]
      })
    );

    render(<App />);
    await userEvent.type(screen.getByLabelText("API 暗号化 key"), payloadEncKey);
    await userEvent.type(screen.getByLabelText("郵便番号"), "1000001");
    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));

    expect(await screen.findByText("東京都")).toBeInTheDocument();
    expect(screen.getByText("total_count")).toBeInTheDocument();
    expect(sessionStorage.getItem("payloadEncryptionKey")).toBe(payloadEncKey);
  });

  it("shows guidance when encrypted API responses arrive without a configured key", async () => {
    sessionStorage.setItem("apiToken", "read-token");
    vi.mocked(fetch).mockResolvedValueOnce(
      await encryptedJsonResponse({
        status: "ok",
        total_count: 1,
        returned_count: 1,
        truncated: false,
        items: []
      })
    );

    render(<App />);
    await userEvent.type(screen.getByLabelText("郵便番号"), "1000001");
    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("AES-GCM key");
  });

  it("shows a decrypt failure when the configured AES-GCM key does not match", async () => {
    sessionStorage.setItem("apiToken", "read-token");
    sessionStorage.setItem("payloadEncryptionKey", wrongPayloadEncKey);
    vi.mocked(fetch).mockResolvedValueOnce(
      await encryptedJsonResponse({
        status: "ok",
        total_count: 1,
        returned_count: 1,
        truncated: false,
        items: []
      })
    );

    render(<App />);
    await userEvent.type(screen.getByLabelText("郵便番号"), "1000001");
    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("復号に失敗しました");
  });

  it("decrypts encrypted API error responses before showing the structured error", async () => {
    sessionStorage.setItem("apiToken", "bad-token");
    sessionStorage.setItem("payloadEncryptionKey", payloadEncKey);
    vi.mocked(fetch).mockResolvedValueOnce(
      await encryptedJsonResponse(
        {
          status: "unauthorized",
          message: "token is invalid"
        },
        { status: 401 }
      )
    );

    render(<App />);
    await userEvent.type(screen.getByLabelText("都道府県"), "東京");
    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("unauthorized");
    expect(screen.getByRole("alert")).toHaveTextContent("token is invalid");
  });

  it("shows authentication failures clearly", async () => {
    sessionStorage.setItem("apiToken", "bad-token");
    vi.mocked(fetch).mockResolvedValueOnce(
      jsonResponse(
        {
          status: "unauthorized",
          message: "token is invalid"
        },
        { status: 401 }
      )
    );

    render(<App />);
    await userEvent.type(screen.getByLabelText("都道府県"), "東京");
    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("unauthorized");
    expect(screen.getByRole("alert")).toHaveTextContent("token is invalid");
  });

  it("renders zero-result and too-many-result search states", async () => {
    sessionStorage.setItem("apiToken", "read-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(
        jsonResponse({
          status: "ok",
          total_count: 0,
          returned_count: 0,
          truncated: false,
          items: []
        })
      )
      .mockResolvedValueOnce(
        jsonResponse({
          status: "too_many_results",
          total_count: 1000,
          returned_count: 20,
          truncated: true,
          items: []
        })
      );

    render(<App />);
    await userEvent.type(screen.getByLabelText("キーワード"), "not-found");
    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));
    expect(await screen.findByText("該当する住所はありません。")).toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText("キーワード"));
    await userEvent.type(screen.getByLabelText("キーワード"), "東京");
    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));
    expect(await screen.findByText("結果が多すぎます。都道府県や市区町村で条件を絞ってください。")).toBeInTheDocument();
  });

  it("shows timeout state from response status and timeout errors", async () => {
    sessionStorage.setItem("apiToken", "read-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(
        jsonResponse({
          status: "timeout",
          total_count: 0,
          returned_count: 0,
          truncated: false,
          items: []
        })
      )
      .mockResolvedValueOnce(
        jsonResponse(
          {
            status: "timeout",
            message: "query timed out"
          },
          { status: 504 }
        )
      );

    render(<App />);
    await userEvent.type(screen.getByLabelText("市区町村"), "千代田区");
    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));
    expect(await screen.findByText("検索がタイムアウトしました。条件を絞って再試行してください。")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "検索実行" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("timeout");
    expect(screen.getByRole("alert")).toHaveTextContent("query timed out");
  });

  it("shows sync status and run history in admin", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 15,
          running: false,
          last_success_at: "2026-06-11T00:00:00Z",
          last_type: "full"
        })
      )
      .mockResolvedValueOnce(
        jsonResponse([
          {
            id: "run-1",
            type: "full",
            status: "success",
            trigger: "manual",
            rows_added: 10,
            rows_updated: 5,
            rows_deleted: 0,
            rows_total: 15,
            started_at: "2026-06-11T00:00:00Z",
            finished_at: "2026-06-11T00:00:01Z",
            error_message: null
          }
        ])
      );

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));

    expect(await screen.findByText("total_addresses")).toBeInTheDocument();
    expect(screen.getAllByText("full").length).toBeGreaterThan(0);
    expect(screen.getByText("success")).toBeInTheDocument();
  });

  it("loads persisted sync run history when the admin page opens", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 42,
          running: false,
          last_success_at: "2026-06-11T00:00:00Z",
          last_type: "diff"
        })
      )
      .mockResolvedValueOnce(
        jsonResponse([
          {
            id: "persisted-run",
            type: "diff",
            status: "success",
            trigger: "schedule",
            rows_added: 1,
            rows_updated: 2,
            rows_deleted: 3,
            rows_total: 6,
            started_at: "2026-06-11T00:00:00Z",
            finished_at: "2026-06-11T00:00:01Z",
            error_message: null
          }
        ])
      );

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));

    expect(await screen.findByText("total_addresses")).toBeInTheDocument();
    expect(screen.getAllByText("diff").length).toBeGreaterThan(0);
    expect(screen.getByText("success")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith(
      "/v1/sync/runs?limit=100&offset=0",
      expect.objectContaining({
        headers: expect.any(Headers)
      })
    );
  });

  it("opens skipped row details from sync history and paginates them", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 42,
          running: false,
          last_success_at: "2026-06-11T00:00:00Z",
          last_type: "full"
        })
      )
      .mockResolvedValueOnce(
        jsonResponse([
          {
            id: "run-skip",
            type: "full",
            status: "success",
            trigger: "manual",
            rows_added: 3,
            rows_updated: 1,
            rows_deleted: 0,
            rows_skipped: 101,
            rows_total: 105,
            started_at: "2026-06-11T00:00:00Z",
            finished_at: "2026-06-11T00:00:01Z",
            error_message: null
          }
        ])
      )
      .mockResolvedValueOnce(jsonResponse(Array.from({ length: 100 }, (_, index) => skippedRow(index + 1))))
      .mockResolvedValueOnce(jsonResponse([skippedRow(101)]));

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));

    await userEvent.click(await screen.findByRole("button", { name: "除外行を表示" }));

    expect(fetch).toHaveBeenCalledWith(
      "/v1/sync/runs/run-skip/skipped?limit=100&offset=0",
      expect.objectContaining({
        headers: expect.any(Headers)
      })
    );
    expect(await screen.findByText("除外行明細")).toBeInTheDocument();
    expect(screen.getByText("東京都 / 千代田区 / 除外町域1")).toBeInTheDocument();
    expect(screen.getAllByText("(?i)町域").length).toBeGreaterThan(0);
    expect(screen.getByText(/\["raw-row-1","東京都"/)).toBeInTheDocument();
    expect(screen.queryByText(/raw-tail-1/)).not.toBeInTheDocument();

    await userEvent.click(screen.getAllByRole("button", { name: "raw を表示" })[0]);
    expect(screen.getByText(/raw-tail-1/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "さらに読み込む" }));

    expect(fetch).toHaveBeenCalledWith(
      "/v1/sync/runs/run-skip/skipped?limit=100&offset=100",
      expect.objectContaining({
        headers: expect.any(Headers)
      })
    );
    expect(await screen.findByText("東京都 / 千代田区 / 除外町域101")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "さらに読み込む" })).not.toBeInTheDocument();
  });

  it("clears skipped row details when the bearer token is removed", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 42,
          running: false,
          last_success_at: "2026-06-11T00:00:00Z",
          last_type: "full"
        })
      )
      .mockResolvedValueOnce(
        jsonResponse([
          {
            id: "run-skip",
            type: "full",
            status: "success",
            trigger: "manual",
            rows_added: 3,
            rows_updated: 1,
            rows_deleted: 0,
            rows_skipped: 1,
            rows_total: 5,
            started_at: "2026-06-11T00:00:00Z",
            finished_at: "2026-06-11T00:00:01Z",
            error_message: null
          }
        ])
      )
      .mockResolvedValueOnce(jsonResponse([skippedRow(1)]));

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));
    await userEvent.click(await screen.findByRole("button", { name: "除外行を表示" }));
    expect(await screen.findByText("東京都 / 千代田区 / 除外町域1")).toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText("Bearer token"));

    expect(screen.queryByText("除外行明細")).not.toBeInTheDocument();
    expect(screen.queryByText("東京都 / 千代田区 / 除外町域1")).not.toBeInTheDocument();
    expect(screen.getByText("同期履歴はまだありません。")).toBeInTheDocument();
  });

  it("separates sync refresh from the selected sync-mode action", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 42,
          running: false,
          last_success_at: "2026-06-11T00:00:00Z",
          last_type: "diff"
        })
      )
      .mockResolvedValueOnce(jsonResponse([]));

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));

    expect(await screen.findByText("total_addresses")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "状態を再読込" })).toBeInTheDocument();
    expect(screen.getByText("現在の状態と最新 100 件の履歴だけを再取得します。")).toBeInTheDocument();
    expect(screen.getByLabelText("同期方式")).toHaveValue("auto");
    expect(screen.getByText("下のボタンは選択した方式で同期を開始します。")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "選択した方式で同期実行" })).toBeInTheDocument();
  });

  it("clears sync status and run history when the bearer token is removed", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 42,
          running: false,
          last_success_at: "2026-06-11T00:00:00Z",
          last_type: "diff"
        })
      )
      .mockResolvedValueOnce(
        jsonResponse([
          {
            id: "persisted-run",
            type: "diff",
            status: "success",
            trigger: "schedule",
            rows_added: 1,
            rows_updated: 2,
            rows_deleted: 3,
            rows_total: 6,
            started_at: "2026-06-11T00:00:00Z",
            finished_at: "2026-06-11T00:00:01Z",
            error_message: null
          }
        ])
      );

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));
    expect(await screen.findByText("success")).toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText("Bearer token"));

    expect(screen.queryByText("total_addresses")).not.toBeInTheDocument();
    expect(screen.queryByText("success")).not.toBeInTheDocument();
    expect(screen.getByText("同期履歴はまだありません。")).toBeInTheDocument();
  });

  it("triggers auto sync and refreshes running state", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 0,
          running: false,
          last_success_at: null,
          last_type: null
        })
      )
      .mockResolvedValueOnce(jsonResponse([]))
      .mockResolvedValueOnce(
        jsonResponse({
          id: "run-auto",
          type: "auto",
          status: "running",
          trigger: "manual",
          rows_added: 0,
          rows_updated: 0,
          rows_deleted: 0,
          rows_total: 0,
          started_at: "2026-06-11T00:00:00Z",
          finished_at: null,
          error_message: null
        })
      )
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 15,
          running: true,
          last_success_at: "2026-06-11T00:00:00Z",
          last_type: "full"
        })
      )
      .mockResolvedValueOnce(
        jsonResponse([
          {
            id: "run-auto",
            type: "auto",
            status: "running",
            trigger: "manual",
            rows_added: 0,
            rows_updated: 0,
            rows_deleted: 0,
            rows_total: 0,
            started_at: "2026-06-11T00:00:00Z",
            finished_at: null,
            error_message: null
          }
        ])
      );

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));
    expect(await screen.findByText("total_addresses")).toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText("同期方式"), "auto");
    await userEvent.click(screen.getByRole("button", { name: "選択した方式で同期実行" }));

    expect(await screen.findByRole("status")).toHaveTextContent("auto");
    expect(screen.getByText("yes")).toBeInTheDocument();
    expect(screen.getAllByText("running").length).toBeGreaterThan(0);
    expect(fetch).toHaveBeenCalledWith(
      "/v1/sync/trigger",
      expect.objectContaining({
        body: JSON.stringify({ type: "auto" })
      })
    );
  });

  it("creates a token and hides one-time plaintext", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 0,
          running: false,
          last_success_at: null,
          last_type: null
        })
      )
      .mockResolvedValueOnce(jsonResponse([]))
      .mockResolvedValueOnce(
        jsonResponse(
          {
            id: "token-1",
            name: "sample",
            prefix: "jdps_1234",
            scope: "read",
            created_at: "2026-06-11T00:00:00Z",
            last_used_at: null,
            revoked_at: null,
            token: "jdps_plaintext_once"
          },
          { status: 201 }
        )
      )
      .mockResolvedValueOnce(
        jsonResponse([
          {
            id: "token-1",
            name: "sample",
            prefix: "jdps_1234",
            scope: "read",
            created_at: "2026-06-11T00:00:00Z",
            last_used_at: null,
            revoked_at: null
          }
        ])
    );

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));
    expect(await screen.findByText("total_addresses")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("名前"), "sample");
    await userEvent.click(screen.getByRole("button", { name: "発行" }));

    expect(await screen.findByText("jdps_plaintext_once")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("一度だけ");
    await userEvent.click(within(screen.getByRole("status")).getByRole("button", { name: "非表示" }));
    expect(screen.queryByText("jdps_plaintext_once")).not.toBeInTheDocument();
  });

  it("groups token action buttons away from token inputs", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 0,
          running: false,
          last_success_at: null,
          last_type: null
        })
      )
      .mockResolvedValueOnce(jsonResponse([]));

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));
    expect(await screen.findByText("Token 管理")).toBeInTheDocument();

    const tokenActions = screen.getByRole("group", { name: "Token 操作" });
    expect(within(tokenActions).getByRole("button", { name: "発行" })).toBeInTheDocument();
    expect(within(tokenActions).getByRole("button", { name: "一覧更新" })).toBeInTheDocument();
  });

  it("saves admin settings and restores defaults", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 0,
          running: false,
          last_success_at: null,
          last_type: null
        })
      )
      .mockResolvedValueOnce(jsonResponse([]))
      .mockResolvedValueOnce(
        jsonResponse(
          settingsBody({
            download_max_retry: { value: 5, default: 3, overridden: true },
            town_skip_regex: { value: "(?i)町域", default: "", overridden: true }
          })
        )
      )
      .mockResolvedValueOnce(jsonResponse(settingsBody()));

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));

    const retryInput = await screen.findByLabelText("リトライ回数");
    await userEvent.clear(retryInput);
    await userEvent.type(retryInput, "5");
    await userEvent.type(screen.getByLabelText("町域名フィルター"), "(?i)町域");
    await userEvent.click(screen.getByRole("button", { name: "保存" }));

    expect(await screen.findByText("設定を保存しました。")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith(
      "/v1/admin/settings",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({
          download_max_retry: 5,
          scrape_full_url: "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip",
          town_skip_regex: "(?i)町域"
        })
      })
    );

    await userEvent.click(screen.getByRole("button", { name: "既定値に戻す" }));

    expect(await screen.findByText("既定値に戻しました。")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith(
      "/v1/admin/settings",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({
          reset_to_default: ["download_max_retry", "scrape_full_url", "town_skip_regex"]
        })
      })
    );
  });

  it("shows backend validation errors for invalid town filter regexes", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 0,
          running: false,
          last_success_at: null,
          last_type: null
        })
      )
      .mockResolvedValueOnce(jsonResponse([]))
      .mockResolvedValueOnce(
        jsonResponse(
          {
            status: "invalid_request",
            message: "町域名フィルターの正規表現が正しくありません。"
          },
          { status: 400 }
        )
      );

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));

    fireEvent.change(await screen.findByLabelText("町域名フィルター"), { target: { value: "[" } });
    await userEvent.click(screen.getByRole("button", { name: "保存" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("町域名フィルターの正規表現が正しくありません。");
    expect(fetch).toHaveBeenCalledWith(
      "/v1/admin/settings",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({
          download_max_retry: 3,
          scrape_full_url: "https://www.post.japanpost.jp/service/search/zipcode/download/utf/zip/utf_ken_all.zip",
          town_skip_regex: "["
        })
      })
    );
  });

  it("validates admin setting URL in Japanese before saving", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 0,
          running: false,
          last_success_at: null,
          last_type: null
        })
      )
      .mockResolvedValueOnce(jsonResponse([]));

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));

    const urlInput = await screen.findByLabelText("全量取得 URL");
    await userEvent.clear(urlInput);
    await userEvent.type(urlInput, "http://example.com/utf_ken_all.zip");
    await userEvent.click(screen.getByRole("button", { name: "保存" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("URL は https で指定してください。");
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it("uploads a csv file and refreshes sync history after success", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 0,
          running: false,
          last_success_at: null,
          last_type: null
        })
      )
      .mockResolvedValueOnce(jsonResponse([]))
      .mockResolvedValueOnce(
        jsonResponse({
          id: "upload-run",
          type: "full",
          status: "success",
          trigger: "upload",
          rows_added: 10,
          rows_updated: 2,
          rows_deleted: 1,
          rows_total: 13,
          started_at: "2026-06-11T00:00:00Z",
          finished_at: "2026-06-11T00:00:02Z",
          error_message: null
        })
      )
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 13,
          running: false,
          last_success_at: "2026-06-11T00:00:02Z",
          last_type: "full"
        })
      )
      .mockResolvedValueOnce(
        jsonResponse([
          {
            id: "upload-run",
            type: "full",
            status: "success",
            trigger: "upload",
            rows_added: 10,
            rows_updated: 2,
            rows_deleted: 1,
            rows_total: 13,
            started_at: "2026-06-11T00:00:00Z",
            finished_at: "2026-06-11T00:00:02Z",
            error_message: null
          }
        ])
      );

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));
    await screen.findByText("ファイルアップロード");

    await userEvent.upload(screen.getByLabelText(/zip\/csv をドラッグ/), new File(["zipcode"], "utf_ken_all.csv", { type: "text/csv" }));
    await userEvent.click(screen.getByRole("button", { name: "アップロード実行" }));

    expect(await screen.findByText("取り込みが完了しました。")).toBeInTheDocument();
    expect(screen.getByText("追加 10 / 更新 2 / 削除 1 / 合計 13")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith(
      "/v1/sync/upload",
      expect.objectContaining({
        method: "POST",
        body: expect.any(FormData)
      })
    );
    expect(fetch).toHaveBeenCalledWith("/v1/sync/runs?limit=100&offset=0", expect.any(Object));
  });

  it("shows structured upload errors in Japanese", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
      .mockResolvedValueOnce(jsonResponse(settingsBody()))
      .mockResolvedValueOnce(
        jsonResponse({
          total_addresses: 0,
          running: false,
          last_success_at: null,
          last_type: null
        })
      )
      .mockResolvedValueOnce(jsonResponse([]))
      .mockResolvedValueOnce(
        jsonResponse(
          {
            status: "csv_format_error",
            message: "UTF-8 の utf_ken_all CSV のみ対応しています。Shift-JIS 版は利用できません。"
          },
          { status: 422 }
        )
      );

    render(<App />);
    await userEvent.click(screen.getByRole("button", { name: "管理" }));
    await screen.findByText("ファイルアップロード");

    await userEvent.upload(screen.getByLabelText(/zip\/csv をドラッグ/), new File(["bad"], "utf_ken_all.csv", { type: "text/csv" }));
    await userEvent.click(screen.getByRole("button", { name: "アップロード実行" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("csv_format_error");
    expect(screen.getByRole("alert")).toHaveTextContent("Shift-JIS 版は利用できません。");
  });
});
