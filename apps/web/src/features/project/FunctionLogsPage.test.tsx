import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { FunctionLogsPage } from "./FunctionLogsPage";

afterEach(() => vi.useRealTimers());

const page = (
  logs: Record<string, unknown>[] = [],
  overrides: Record<string, unknown> = {},
) => ({
  logs,
  olderCursor: "",
  newerCursor: "new",
  hasMoreNewer: false,
  health: { status: "healthy", dropped: 0, rejected: 0, detail: "" },
  serverTime: "2026-09-04T12:00:00Z",
  ...overrides,
});
const log = (id: string, timestamp: string, message = id, level = "info") => ({
  id,
  projectId: "project one",
  functionName: "snow/雪",
  executionId: `exec-${id}`,
  eventType: "Log",
  message,
  timestamp,
  ingestedAt: timestamp,
  level,
  truncated: false,
});

function setup(
  fetcher: typeof fetch,
  route = "/projects/project%20one/functions/snow%2F%E9%9B%AA/logs",
) {
  vi.stubGlobal("fetch", vi.fn(fetcher));
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[route]}>
        <Routes>
          <Route
            path="/projects/:projectId/functions/:functionName/logs"
            element={<FunctionLogsPage />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

it("encodes the API path once, links back, and renders newest-first log rows", async () => {
  const requests: string[] = [];
  setup(async (input) => {
    requests.push(String(input));
    return new Response(
      JSON.stringify(
        page([
          log("old", "2026-09-04T10:00:00Z", "line 1\n  line 2"),
          log("new", "2026-09-04T11:00:00Z", "latest", "error"),
        ]),
      ),
      { status: 200, headers: { "Content-Type": "application/json" } },
    );
  });
  expect(
    await screen.findByRole("heading", { name: /snow\/雪.*logs/i }),
  ).toBeVisible();
  expect(
    screen.getByRole("link", { name: /back to functions/i }),
  ).toHaveAttribute("href", "/projects/project%20one/functions");
  expect(requests[0]).toBe(
    "/api/projects/project%20one/functions/snow%2F%E9%9B%AA/logs?limit=200",
  );
  await screen.findByText("latest");
  const rows = screen.getAllByRole("row").slice(1);
  expect(within(rows[0]).getByText("latest")).toBeVisible();
  expect(within(rows[1]).getByText(/line 1/)).toHaveClass(
    "function-log-message",
  );
});

it("debounces filters, caps search at 256 UTF-8 bytes, and resets to a base request", async () => {
  vi.useFakeTimers();
  const requests: string[] = [];
  setup(async (input) => {
    requests.push(String(input));
    return new Response(JSON.stringify(page()), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  const input = screen.getByRole("searchbox", { name: /search logs/i });
  act(() => fireInput(input, "界".repeat(86) + "a"));
  expect(input).toHaveValue("界".repeat(85) + "a");
  expect(requests).toHaveLength(1);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(299);
  });
  expect(requests).toHaveLength(1);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
    await Promise.resolve();
  });
  expect(
    requests.some((request) =>
      request.includes(`search=${encodeURIComponent("界".repeat(85) + "a")}`),
    ),
  ).toBe(true);
  vi.useRealTimers();
  const user = userEvent.setup();
  await user.click(screen.getByRole("combobox", { name: /log level/i }));
  await user.click(await screen.findByRole("option", { name: "Error" }));
  await screen.findByText("Log collection healthy");
  expect(requests.at(-1)).toContain("level=error");
  expect(requests.at(-1)).not.toContain("after=");
});

it("polls with the newer cursor, pauses and resumes, refreshes incrementally, loads older, and deduplicates", async () => {
  vi.useFakeTimers();
  const requests: string[] = [];
  let call = 0;
  setup(async (input) => {
    requests.push(String(input));
    call += 1;
    const response =
      call === 1
        ? page([log("one", "2026-09-04T10:00:00Z")], {
            newerCursor: "new 1",
            olderCursor: "old/1",
          })
        : String(input).includes("before=")
          ? page([
              log("older", "2026-09-04T09:00:00Z"),
              log("one", "2026-09-04T10:00:00Z"),
            ])
          : page(
              [
                log("two", "2026-09-04T11:00:00Z"),
                log("one", "2026-09-04T10:00:00Z"),
              ],
              { newerCursor: "new 2", olderCursor: "old/1" },
            );
    return new Response(JSON.stringify(response), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(screen.getByText("one")).toBeVisible();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000);
    await Promise.resolve();
  });
  await flushAsync();
  expect(requests.some((request) => request.includes("after=new+1"))).toBe(true);
  expect(screen.getAllByText(/^(one|two)$/)).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: /pause/i }));
  const pausedCount = requests.length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(10000);
  });
  expect(requests).toHaveLength(pausedCount);
  fireEvent.click(screen.getByRole("button", { name: /resume/i }));
  fireEvent.click(screen.getByRole("button", { name: /^refresh$/i }));
  await flushAsync();
  expect(requests.filter((request) => request.includes("after=new+2")).length).toBeGreaterThan(0);
  fireEvent.click(screen.getByRole("button", { name: /load older/i }));
  await flushAsync();
  expect(requests.some((request) => request.includes("before=old%2F1"))).toBe(true);
  expect(screen.getAllByText(/^(one|two|older)$/)).toHaveLength(3);
  vi.useRealTimers();
});

it.each([
  ["not_installed", /not installed/i],
  ["offline", /offline/i],
  ["incompatible", /incompatible/i],
  ["dropped", /3.*dropped|dropped.*3/i],
  ["storage_error", /storage/i],
  ["disabled", /disabled/i],
])(
  "shows %s collection health while retaining records",
  async (status, expected) => {
    setup(
      async () =>
        new Response(
          JSON.stringify(
            page([log("kept", "2026-09-04T10:00:00Z")], {
              health: { status, dropped: 3, rejected: 2, detail: "" },
            }),
          ),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
    );
    expect(await screen.findByText("kept")).toBeVisible();
    expect(screen.getByText(expected)).toBeVisible();
  },
);

it("distinguishes healthy empty and retryable initial errors, and retains rows on refetch error", async () => {
  let call = 0;
  setup(async () => {
    call += 1;
    if (call === 1)
      return new Response(
        JSON.stringify(page([log("retained", "2026-09-04T10:00:00Z")])),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    return new Response(JSON.stringify({ error: { message: "temporary" } }), {
      status: 503,
      headers: { "Content-Type": "application/json" },
    });
  });
  expect(await screen.findByText("retained")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: /^refresh$/i }));
  expect(await screen.findByText(/could not refresh/i)).toBeVisible();
  expect(screen.getByText("retained")).toBeVisible();
});

it("shows the healthy empty state and a retry control for an initial error", async () => {
  const empty = setup(async () => new Response(JSON.stringify(page()), { status: 200, headers: { "Content-Type": "application/json" } }));
  expect(await screen.findByText("No retained logs.")).toBeVisible();
  empty.unmount();
  setup(async () => new Response(JSON.stringify({ error: { message: "unavailable" } }), { status: 503, headers: { "Content-Type": "application/json" } }));
  expect(await screen.findByRole("button", { name: "Retry" })).toBeVisible();
});

it("stops polling after unmount", async () => {
  vi.useFakeTimers();
  const requests: string[] = [];
  const view = setup(async (input) => { requests.push(String(input)); return new Response(JSON.stringify(page()), { status: 200, headers: { "Content-Type": "application/json" } }); });
  await flushAsync();
  view.unmount();
  const count = requests.length;
  await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
  expect(requests).toHaveLength(count);
});

it("aborts a pending initial request on unmount", async () => {
  let signal: AbortSignal | undefined;
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => { resolve = done; });
  const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
  const view = setup(async (_input, init) => { signal = init?.signal as AbortSignal; return pending; });
  await flushAsync();
  view.unmount();
  expect(signal?.aborted).toBe(true);
  resolve(new Response(JSON.stringify(page()), { status: 200, headers: { "Content-Type": "application/json" } }));
  await flushAsync();
  expect(error).not.toHaveBeenCalled();
  error.mockRestore();
});

it("aborts a pending older request on unmount", async () => {
  let olderSignal: AbortSignal | undefined;
  let resolveOlder!: (response: Response) => void;
  const pendingOlder = new Promise<Response>((done) => { resolveOlder = done; });
  const view = setup(async (input, init) => String(input).includes("before=") ? (olderSignal = init?.signal as AbortSignal, pendingOlder) : new Response(JSON.stringify(page([log("base", "2026-09-04T10:00:00Z")], { olderCursor: "old" })), { status: 200, headers: { "Content-Type": "application/json" } }));
  await screen.findByText("base");
  fireEvent.click(screen.getByRole("button", { name: /load older/i }));
  await flushAsync();
  view.unmount();
  expect(olderSignal?.aborted).toBe(true);
  resolveOlder(new Response(JSON.stringify(page()), { status: 200, headers: { "Content-Type": "application/json" } }));
});

it("aborts a pending incremental refetch on unmount", async () => {
  let refetchSignal: AbortSignal | undefined;
  let resolveRefetch!: (response: Response) => void;
  const pendingRefetch = new Promise<Response>((done) => { resolveRefetch = done; });
  const view = setup(async (input, init) => String(input).includes("after=") ? (refetchSignal = init?.signal as AbortSignal, pendingRefetch) : new Response(JSON.stringify(page([log("base", "2026-09-04T10:00:00Z")], { newerCursor: "newer" })), { status: 200, headers: { "Content-Type": "application/json" } }));
  await screen.findByText("base");
  fireEvent.click(screen.getByRole("button", { name: /^refresh$/i }));
  await flushAsync();
  view.unmount();
  expect(refetchSignal?.aborted).toBe(true);
  resolveRefetch(new Response(JSON.stringify(page()), { status: 200, headers: { "Content-Type": "application/json" } }));
});

it("keeps newer poll health and records when an older page resolves last", async () => {
  vi.useFakeTimers();
  let resolveOlder!: (response: Response) => void;
  const pendingOlder = new Promise<Response>((done) => { resolveOlder = done; });
  setup(async (input) => {
    const path = String(input);
    if (path.includes("before=")) return pendingOlder;
    if (path.includes("after=")) return new Response(JSON.stringify(page([log("new", "2026-09-04T11:00:00Z")], { newerCursor: "next", health: { status: "dropped", dropped: 7, rejected: 0, detail: "" } })), { status: 200, headers: { "Content-Type": "application/json" } });
    return new Response(JSON.stringify(page([log("base", "2026-09-04T10:00:00Z")], { olderCursor: "old", newerCursor: "first" })), { status: 200, headers: { "Content-Type": "application/json" } });
  });
  await flushAsync();
  fireEvent.click(screen.getByRole("button", { name: /load older/i }));
  await act(async () => { await vi.advanceTimersByTimeAsync(5001); });
  await flushAsync();
  expect(screen.getByText(/7 log events were dropped/i)).toBeVisible();
  resolveOlder(new Response(JSON.stringify(page([log("old", "2026-09-04T09:00:00Z")], { olderCursor: "", health: { status: "offline", dropped: 0, rejected: 0, detail: "stale" } })), { status: 200, headers: { "Content-Type": "application/json" } }));
  await flushAsync();
  expect(screen.getByText(/7 log events were dropped/i)).toBeVisible();
  expect(screen.queryByText(/offline/i)).not.toBeInTheDocument();
  expect(screen.getAllByText(/^(old|base|new)$/)).toHaveLength(3);
});

it("does not merge a stale filter response into the current filter key", async () => {
  let resolveError!: (response: Response) => void;
  const pendingError = new Promise<Response>((done) => { resolveError = done; });
  setup(async (input) => {
    const path = String(input);
    if (path.includes("level=error")) return pendingError;
    if (path.includes("level=warn")) return new Response(JSON.stringify(page([log("warn-only", "2026-09-04T11:00:00Z", "warn-only", "warn")])), { status: 200, headers: { "Content-Type": "application/json" } });
    return new Response(JSON.stringify(page()), { status: 200, headers: { "Content-Type": "application/json" } });
  });
  const user = userEvent.setup();
  await screen.findByText("No retained logs.");
  await user.click(screen.getByRole("combobox", { name: /log level/i }));
  await user.click(await screen.findByRole("option", { name: "Error" }));
  await user.click(screen.getByRole("combobox", { name: /log level/i }));
  await user.click(await screen.findByRole("option", { name: "Warn" }));
  expect(await screen.findByText("warn-only")).toBeVisible();
  resolveError(new Response(JSON.stringify(page([log("stale-error", "2026-09-04T12:00:00Z", "stale-error", "error")])), { status: 200, headers: { "Content-Type": "application/json" } }));
  await flushAsync();
  expect(screen.queryByText("stale-error")).not.toBeInTheDocument();
  expect(screen.getByText("warn-only")).toBeVisible();
});

it("drains every contiguous newer page without skipping a 450-record burst", async () => {
  const requestedAfter: string[] = [];
  setup(async (input) => {
    const after = new URL(String(input), "http://local").searchParams.get("after") ?? "";
    requestedAfter.push(after);
    if (!after) return new Response(JSON.stringify(page([log("base", "2026-09-04T00:00:00Z")], { newerCursor: "cursor-0" })), { status: 200, headers: { "Content-Type": "application/json" } });
    const start = after === "cursor-0" ? 1 : after === "cursor-200" ? 201 : 401;
    const end = Math.min(start + 199, 450);
    const logs = Array.from({ length: end - start + 1 }, (_, index) => log(`event-${start + index}`, new Date(Date.UTC(2026, 8, 4) + (start + index) * 1000).toISOString()));
    return new Response(JSON.stringify(page(logs, { newerCursor: `cursor-${end}`, hasMoreNewer: end < 450 })), { status: 200, headers: { "Content-Type": "application/json" } });
  });
  await screen.findByText("base");
  fireEvent.click(screen.getByRole("button", { name: /^refresh$/i }));
  await screen.findByText("event-450");
  expect(requestedAfter).toEqual(["", "cursor-0", "cursor-200", "cursor-400"]);
  expect(screen.getAllByRole("row")).toHaveLength(452);
});

it("sorts RFC3339 nanoseconds newest-first with ID as the stable final tie-breaker", async () => {
  setup(async () => new Response(JSON.stringify(page([
    log("exact", "2026-09-04T10:00:00Z"),
    log("nano-a", "2026-09-04T10:00:00.123456789Z"),
    log("nano-b", "2026-09-04T10:00:00.123456789Z"),
    log("micro", "2026-09-04T10:00:00.123456001Z"),
  ])), { status: 200, headers: { "Content-Type": "application/json" } }));
  await screen.findByText("exact");
  expect(screen.getAllByRole("row").slice(1).map((row) => within(row).getByText(/^(exact|nano-a|nano-b|micro)$/).textContent)).toEqual(["nano-b", "nano-a", "micro", "exact"]);
});

it("caps rendered retained logs at 2,000 and stops older pagination at the cap", async () => {
  const logs = Array.from({ length: 2_050 }, (_, index) => log(`bounded-${index}`, `2026-09-04T10:00:${String(index % 60).padStart(2, "0")}.${String(index).padStart(9, "0")}Z`));
  setup(async () => new Response(JSON.stringify(page(logs, { olderCursor: "older" })), { status: 200, headers: { "Content-Type": "application/json" } }));
  expect(await screen.findByText(/Showing latest 2,000/i)).toBeVisible();
  expect(screen.getAllByRole("row")).toHaveLength(2_001);
  expect(screen.queryByRole("button", { name: /load older/i })).not.toBeInTheDocument();
});

it.each(["offline", "not_installed"])("shows explicit no-records copy when health is %s", async (status) => {
  setup(async () => new Response(JSON.stringify(page([], { health: { status, dropped: 0, rejected: 0, detail: "" } })), { status: 200, headers: { "Content-Type": "application/json" } }));
  expect(await screen.findByText("No retained logs.")).toBeVisible();
});

function fireInput(input: HTMLElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )!.set!;
  setter.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

async function flushAsync() {
  await act(async () => {
    for (let index = 0; index < 5; index += 1) {
      await Promise.resolve();
      if (vi.isFakeTimers()) await vi.advanceTimersByTimeAsync(1);
    }
  });
}
