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
  fireEvent.change(screen.getByRole("combobox", { name: /log level/i }), { target: { value: "error" } });
  await flushAsync();
  expect(requests.at(-1)).toContain("level=error");
  expect(requests.at(-1)).not.toContain("after=");
  vi.useRealTimers();
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
