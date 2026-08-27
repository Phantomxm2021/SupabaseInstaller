import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ConfigurationPage } from "./ConfigurationPage";
import { defaultConfiguration } from "../projects/projectSchema";
import {
  AuthenticationWorkspace,
  EmailsRoute,
  SignInProvidersRoute,
} from "../authentication/AuthenticationWorkspace";

it("renders the installed project configuration workspace from the redacted snapshot", async () => {
  const configuration = defaultConfiguration("LIGHTWEIGHT");
  configuration.general = {
    domain: "bee.example.com",
    siteUrl: "https://example.com",
    supabaseVersion: "self-hosted/v0.8.0",
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            projectId: "bee",
            revision: 4,
            lastGoodRevision: 4,
            configuration,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter
        initialEntries={["/projects/bee/configuration?section=database"]}
      >
        <Routes>
          <Route
            path="/projects/:projectId/configuration"
            element={<ConfigurationPage />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(
    await screen.findByText("Database", {
      selector: '[data-slot="card-title"]',
    }),
  ).toBeVisible();
  expect(screen.getByLabelText("Maximum connections")).toBeVisible();
  const settings = screen.getByRole("navigation", {
    name: "Project settings navigation",
  });
  expect(within(settings).getAllByRole("link")).toHaveLength(9);
  expect(
    within(settings).getByRole("heading", { name: "Project Settings" }),
  ).toBeVisible();
  expect(
    within(settings).getByRole("heading", { name: "PROJECT" }),
  ).toBeVisible();
  expect(
    within(settings).getByRole("heading", { name: "INFRASTRUCTURE" }),
  ).toBeVisible();
  expect(
    within(settings).getByRole("link", { name: "Database" }),
  ).toHaveAttribute("aria-current", "page");
  expect(
    within(settings).getByRole("link", { name: "API & Secrets" }),
  ).toHaveAttribute("href", "/projects/bee/configuration?section=secrets");
});

function redactedSnapshot(domain = "bee.example.com") {
  const configuration = defaultConfiguration("LIGHTWEIGHT");
  configuration.general = {
    domain,
    siteUrl: "https://example.com",
    supabaseVersion: "self-hosted/v0.8.0",
  };
  const redacted = JSON.parse(
    JSON.stringify(configuration),
  ) as typeof configuration;
  delete (redacted.auth as unknown as { redirectUrls?: string[] }).redirectUrls;
  delete (redacted.auth as unknown as { oauth?: Record<string, unknown> }).oauth
    ?.google;
  delete (redacted.auth.phone as unknown as { provider?: string }).provider;
  delete (redacted.auth.phone as unknown as { fields?: Record<string, string> })
    .fields;
  delete (redacted.functions as unknown as { variables?: unknown[] }).variables;
  delete (redacted.database as unknown as { extensions?: string[] }).extensions;
  return {
    projectId: "bee",
    revision: 4,
    lastGoodRevision: 4,
    configuration: redacted,
  };
}

function renderConfiguration(section = "general") {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter
        initialEntries={[`/projects/bee/configuration?section=${section}`]}
      >
        <Routes>
          <Route
            path="/projects/:projectId/configuration"
            element={<ConfigurationPage />}
          />
          <Route
            path="/projects/:projectId/authentication"
            element={<AuthenticationWorkspace />}
          >
            <Route
              path="sign-in-providers"
              element={<SignInProvidersRoute />}
            />
            <Route path="emails" element={<EmailsRoute />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

it("keeps dirty input when preview is dismissed with Keep editing", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PATCH")
        return new Response(
          JSON.stringify({
            projectId: "bee",
            operationId: "op-1",
            revision: 5,
          }),
          { status: 202 },
        );
      return new Response(JSON.stringify(redactedSnapshot()), { status: 200 });
    }),
  );
  renderConfiguration();
  const domain = await screen.findByLabelText("Domain");
  await user.clear(domain);
  await user.type(domain, "edited.example.com");
  await user.click(screen.getByRole("button", { name: "Save General" }));
  expect(await screen.findByRole("alertdialog")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Keep editing" }));
  expect(screen.getByLabelText("Domain")).toHaveValue("edited.example.com");
});

it("preserves dirty fields on 409 and only Reload resets to new server data", async () => {
  const user = userEvent.setup();
  let getCount = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PATCH")
        return new Response(
          JSON.stringify({
            error: { code: "CONFIGURATION_STALE", message: "stale" },
          }),
          { status: 409 },
        );
      getCount += 1;
      return new Response(
        JSON.stringify(
          redactedSnapshot(
            getCount > 1 ? "server.example.com" : "bee.example.com",
          ),
        ),
        { status: 200 },
      );
    }),
  );
  renderConfiguration();
  const domain = await screen.findByLabelText("Domain");
  await user.clear(domain);
  await user.type(domain, "edited.example.com");
  await user.click(screen.getByRole("button", { name: "Save General" }));
  await user.click(screen.getByRole("button", { name: "Confirm and apply" }));
  expect(
    await screen.findByText(
      "This configuration is stale. Your dirty fields are preserved.",
    ),
  ).toBeVisible();
  expect(screen.getByLabelText("Domain")).toHaveValue("edited.example.com");
  await user.click(screen.getByRole("button", { name: "Reload" }));
  await waitFor(() =>
    expect(screen.getByLabelText("Domain")).toHaveValue("server.example.com"),
  );
});

it("renders authoritative API field errors", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PATCH")
        return new Response(
          JSON.stringify({
            error: {
              code: "INVALID_CONFIGURATION",
              message: "invalid",
              fields: { domain: "Domain is already used" },
            },
          }),
          { status: 422 },
        );
      return new Response(JSON.stringify(redactedSnapshot()), { status: 200 });
    }),
  );
  renderConfiguration();
  const domain = await screen.findByLabelText("Domain");
  await user.clear(domain);
  await user.type(domain, "edited.example.com");
  await user.click(screen.getByRole("button", { name: "Save General" }));
  await user.click(screen.getByRole("button", { name: "Confirm and apply" }));
  expect(
    await screen.findByText(/domain: Domain is already used/),
  ).toBeVisible();
  expect(screen.getByLabelText("Domain")).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(screen.getByText("Domain is already used")).toBeVisible();
});

it.each([
  ["auth", "Sign In / Providers"],
  ["oauth", "Sign In / Providers"],
  ["smtp", "Emails"],
] as const)(
  "redirects legacy %s configuration routes to the Authentication workspace",
  async (section, heading) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify(redactedSnapshot()), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );
    renderConfiguration(section);
    expect(await screen.findByRole("heading", { name: heading })).toBeVisible();
    expect(
      screen.getByRole("navigation", { name: "Authentication navigation" }),
    ).toBeVisible();
  },
);

it("closes only public dependents when Gateway is disabled", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("PointerEvent", MouseEvent);
  let patchBody = "";
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PATCH") {
        patchBody = String(init.body);
        return new Response(
          JSON.stringify({
            projectId: "bee",
            operationId: "op-services",
            revision: 5,
          }),
          { status: 202 },
        );
      }
      const snapshot = redactedSnapshot();
      snapshot.configuration.services = {
        ...defaultConfiguration("FULL").services,
        supavisor: true,
        logs: true,
        vector: true,
        directDb: true,
      };
      return new Response(JSON.stringify(snapshot), { status: 200 });
    }),
  );
  renderConfiguration("services");
  await user.click(
    await screen.findByRole("switch", { name: "Envoy Gateway" }),
  );
  expect(screen.getByRole("switch", { name: "Auth" })).toHaveAttribute(
    "data-unchecked",
  );
  expect(screen.getByRole("switch", { name: "PostgREST" })).toHaveAttribute(
    "data-unchecked",
  );
  expect(screen.getByRole("switch", { name: "Supavisor" })).toHaveAttribute(
    "data-checked",
  );
  expect(
    screen.getByRole("switch", { name: "Logs / Logflare" }),
  ).toHaveAttribute("data-checked");
  expect(
    screen.getByRole("switch", { name: "Direct PostgreSQL port" }),
  ).toHaveAttribute("data-checked");
  expect(screen.getByRole("switch", { name: "postgres-meta" })).toHaveAttribute(
    "data-checked",
  );
  await user.click(screen.getByRole("button", { name: "Save Services" }));
  await user.click(screen.getByRole("button", { name: "Confirm and apply" }));
  await waitFor(() => expect(patchBody).toContain('"postgresMeta":true'));
});

it("enables Studio with Gateway and postgres-meta, then persists the closure", async () => {
  const user = userEvent.setup();
  let patchBody = "";
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PATCH") {
        patchBody = String(init.body);
        return new Response(
          JSON.stringify({
            projectId: "bee",
            operationId: "op-studio",
            revision: 5,
          }),
          { status: 202 },
        );
      }
      const snapshot = redactedSnapshot();
      snapshot.configuration.services = {
        ...defaultConfiguration("LIGHTWEIGHT").services,
        gateway: false,
        studio: false,
        postgresMeta: false,
      };
      return new Response(JSON.stringify(snapshot), { status: 200 });
    }),
  );
  renderConfiguration("services");
  await user.click(await screen.findByRole("switch", { name: "Studio" }));
  expect(screen.getByRole("switch", { name: "Envoy Gateway" })).toHaveAttribute(
    "data-checked",
  );
  expect(screen.getByRole("switch", { name: "postgres-meta" })).toHaveAttribute(
    "data-checked",
  );
  await user.click(screen.getByRole("button", { name: "Save Services" }));
  await user.click(screen.getByRole("button", { name: "Confirm and apply" }));
  await waitFor(() => expect(patchBody).toContain('"gateway":true'));
  expect(patchBody).toContain('"postgresMeta":true');
});

it("does not submit Services when a toggle is returned to its baseline", async () => {
  const user = userEvent.setup();
  let patchCount = 0;
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PATCH") patchCount += 1;
      return new Response(JSON.stringify(redactedSnapshot()), {
        status: init?.method === "PATCH" ? 202 : 200,
      });
    }),
  );
  renderConfiguration("services");
  const directDb = await screen.findByRole("switch", {
    name: "Direct PostgreSQL port",
  });
  await user.click(directDb);
  expect(screen.getByRole("button", { name: "Save Services" })).toBeEnabled();
  await user.click(directDb);
  expect(screen.getByRole("button", { name: "Save Services" })).toBeDisabled();
  expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  expect(patchCount).toBe(0);
});

it("keeps postgres-meta when Studio is disabled and persists the independent intent", async () => {
  const user = userEvent.setup();
  let patchBody = "";
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PATCH") {
        patchBody = String(init.body);
        return new Response(
          JSON.stringify({
            projectId: "bee",
            operationId: "op-studio-off",
            revision: 5,
          }),
          { status: 202 },
        );
      }
      const snapshot = redactedSnapshot();
      snapshot.configuration.services = {
        ...defaultConfiguration("LIGHTWEIGHT").services,
        gateway: true,
        studio: true,
        postgresMeta: true,
      };
      return new Response(JSON.stringify(snapshot), { status: 200 });
    }),
  );
  renderConfiguration("services");
  await user.click(await screen.findByRole("switch", { name: "Studio" }));
  expect(screen.getByRole("switch", { name: "postgres-meta" })).toHaveAttribute(
    "data-checked",
  );
  await user.click(screen.getByRole("button", { name: "Save Services" }));
  await user.click(screen.getByRole("button", { name: "Confirm and apply" }));
  await waitFor(() => expect(patchBody).toContain('"studio":false'));
  expect(patchBody).toContain('"postgresMeta":true');
});

it.each([
  ["general", "Domain", "edited.example.com", "Save General"],
  ["network", "Gateway", "Kong (advanced)", "Save Gateway & Network"],
  ["pooler", "Pool size", "21", "Save Connection Pooler"],
] as const)(
  "renders metadata-only preview for disabled %s owner after editing",
  async (section, field, value, saveLabel) => {
    const user = userEvent.setup();
    vi.stubGlobal("PointerEvent", MouseEvent);
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        const snapshot = redactedSnapshot();
        snapshot.configuration.services = {
          ...defaultConfiguration("LIGHTWEIGHT").services,
          gateway: false,
          auth: false,
          studio: false,
          supavisor: false,
        };
        return new Response(JSON.stringify(snapshot), { status: 200 });
      }),
    );
    renderConfiguration(section);
    if (section === "network") {
      const gateway = await screen.findByRole("combobox", { name: "Gateway" });
      await user.click(gateway);
      await waitFor(() => expect(screen.getByRole("listbox")).toBeVisible());
      await user.click(screen.getByText("Kong (advanced)"));
    } else {
      const control = await screen.findByLabelText(field);
      await user.clear(control);
      await user.type(control, value);
    }
    await user.click(screen.getByRole("button", { name: saveLabel }));
    expect(
      await screen.findByText("Configuration metadata only"),
    ).toBeVisible();
    expect(screen.getByText("No runtime restart expected")).toBeVisible();
  },
);
