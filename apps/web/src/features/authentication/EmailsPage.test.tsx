import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { EmailsPage } from "./EmailsPage";
import type { AuthenticationWorkspaceContext } from "./AuthenticationWorkspace";
import type { Services } from "../../api/types";
import { defaultConfiguration } from "../projects/projectSchema";

const smtp = {
  enabled: false,
  host: "",
  port: 587,
  username: "",
  passwordSet: false,
  password: { action: "" as const },
  senderEmail: "",
  senderName: "",
};
const context: AuthenticationWorkspaceContext = {
  projectId: "bee",
  revision: 1,
  general: {
    domain: "bee.example.test",
    siteUrl: "https://bee.example.test",
    supabaseVersion: "2.0.0",
  },
  services: { auth: true } as Services,
  auth: { ...defaultConfiguration().auth, smtp },
  requestSave: vi.fn(),
};

describe("EmailsPage", () => {
  it("renders authentication and security template groups with navigable rows", async () => {
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <EmailsPage context={context} />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(screen.getByRole("heading", { name: "Emails" })).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Emails" }).closest("main"),
    ).toHaveClass("auth-emails-page");
    expect(screen.getByRole("tab", { name: "Templates" })).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /Confirm sign up/i }),
    ).toHaveAttribute(
      "href",
      "/projects/bee/authentication/emails/confirm-signup",
    );
    expect(
      screen.getByRole("heading", { name: "Security" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("switch", {
        name: "Enable password changed notification",
      }),
    ).toHaveAttribute("aria-checked", "false");
    const save = screen.getByRole("button", { name: "Save changes" });
    expect(save).toBeDisabled();
    expect(save.closest(".auth-settings-card")).toHaveAttribute(
      "aria-label",
      "Security templates",
    );
    expect(screen.getByRole("link", { name: /Confirm sign up/i })).toHaveClass(
      "auth-template-row",
    );
  });

  it("shows an SMTP form and queues a dirty SMTP update through the workspace confirmation path", async () => {
    const user = userEvent.setup();
    const requestSave = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <EmailsPage
            context={{
              ...context,
              requestSave,
              auth: { ...context.auth, smtp: { ...smtp, enabled: true } },
            }}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    await user.click(screen.getByRole("tab", { name: "SMTP Settings" }));
    expect(
      screen.getByRole("switch", { name: "Enable custom SMTP" }),
    ).toBeInTheDocument();
    await user.type(
      screen.getByLabelText("Sender email address"),
      "no-reply@bee.example.test",
    );
    await user.type(screen.getByLabelText("Sender name"), "Bee");
    await user.type(screen.getByLabelText("Host"), "smtp.bee.example.test");
    await user.clear(screen.getByLabelText("Port number"));
    await user.type(screen.getByLabelText("Port number"), "465");
    await user.type(screen.getByLabelText("Username"), "bee");
    await user.type(screen.getByLabelText("Password"), "secret");
    await user.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(requestSave).toHaveBeenCalledTimes(1));
    expect(requestSave.mock.calls[0][0]).toMatchObject({
      section: "smtp",
      value: {
        enabled: true,
        host: "smtp.bee.example.test",
        port: 465,
        senderEmail: "no-reply@bee.example.test",
      },
    });
  });

  it("asks before discarding dirty SMTP fields while changing tabs", async () => {
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <EmailsPage
            context={{
              ...context,
              auth: { ...context.auth, smtp: { ...smtp, enabled: true } },
            }}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    await user.click(screen.getByRole("tab", { name: "SMTP Settings" }));
    await user.type(screen.getByLabelText("Host"), "smtp.bee.example.test");
    await user.click(screen.getByRole("tab", { name: "Templates" }));
    expect(screen.getByRole("alertdialog")).toHaveTextContent(
      "Discard SMTP changes?",
    );
    await user.click(screen.getByRole("button", { name: "Discard changes" }));
    expect(
      screen.getByRole("link", { name: /Confirm sign up/i }),
    ).toBeInTheDocument();
  });
});
