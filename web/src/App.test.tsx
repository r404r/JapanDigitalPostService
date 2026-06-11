import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";

const jsonResponse = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "Content-Type": "application/json" },
    ...init
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

  it("clears sync status and run history when the bearer token is removed", async () => {
    sessionStorage.setItem("apiToken", "admin-token");
    vi.mocked(fetch)
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
    await userEvent.click(screen.getByRole("button", { name: "同期実行" }));

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
});
