import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { EmailTemplateEditorPage } from "./EmailTemplateEditorPage";
import type { AuthenticationWorkspaceContext } from "./AuthenticationWorkspace";
import type { Services } from "../../api/types";
import { defaultMailerConfiguration } from "../projects/projectSchema";

const defaults = defaultMailerConfiguration();
const context = {
  projectId: "bee",
  revision: 3,
  general: {
    domain: "bee.example.test",
    siteUrl: "https://bee.example.test",
    supabaseVersion: "2.0.0",
  },
  services: { auth: true } as Services,
  requestSave: vi.fn(),
  auth: {
    mailer: {
      ...defaults,
      templates: {
        ...defaults.templates,
        confirmation: {
          subject: "Confirm your email address",
          body: "<h1>Confirm your email address</h1>",
        },
      },
    },
  },
} as unknown as AuthenticationWorkspaceContext;

describe("EmailTemplateEditorPage", () => {
  it("uses explicit template layout hooks instead of page spacing utilities", () => {
    render(
      <MemoryRouter>
        <EmailTemplateEditorPage context={context} templateKey="confirm-signup" />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "Confirm sign up" }).closest("main")).not.toHaveClass(
      "space-y-10",
    );
    expect(screen.getByRole("heading", { name: "Template" }).closest("section")).toHaveClass(
      "auth-template-section",
    );
  });

  it("edits the visible source template and persists its HTML body", async () => {
    const user = userEvent.setup();
    const requestSave = vi.fn();
    render(
      <MemoryRouter>
        <EmailTemplateEditorPage
          context={{ ...context, requestSave }}
          templateKey="confirm-signup"
        />
      </MemoryRouter>,
    );
    expect(screen.getByRole("link", { name: "Docs" })).toHaveAttribute(
      "href",
      expect.stringContaining("/auth-email-templates"),
    );
    expect(screen.getByLabelText("HTML body")).toBeInTheDocument();
    await user.clear(screen.getByLabelText("HTML body"));
    await user.type(screen.getByLabelText("HTML body"), "<h1>Welcome</h1>");
    await user.click(screen.getByRole("button", { name: "Save changes" }));
    expect(requestSave).toHaveBeenCalledWith(
      expect.objectContaining({
        value: expect.objectContaining({
          mailer: expect.objectContaining({
            templates: expect.objectContaining({
              confirmation: expect.objectContaining({
                body: "<h1>Welcome</h1>",
              }),
            }),
          }),
        }),
      }),
    );
  });

  it("switches between source and the current HTML preview, then resets", async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <EmailTemplateEditorPage
          context={context}
          templateKey="confirm-signup"
        />
      </MemoryRouter>,
    );
    await user.click(screen.getByRole("button", { name: "Preview" }));
    expect(screen.getByTitle("Email template preview")).toHaveAttribute(
      "sandbox",
    );
    expect(screen.getByTitle("Email template preview")).toHaveAttribute(
      "srcdoc",
      "<h1>Confirm your email address</h1>",
    );
    await user.click(screen.getByRole("button", { name: "Source" }));
    await user.click(screen.getByRole("button", { name: "Reset template" }));
    expect(screen.getByLabelText("HTML body")).toHaveValue(
      defaults.templates.confirmation.body,
    );
  });

  it("does not save notification configuration before an explicit edit and save", () => {
    const requestSave = vi.fn();
    render(
      <MemoryRouter>
        <EmailTemplateEditorPage
          context={{ ...context, requestSave }}
          templateKey="password-changed"
        />
      </MemoryRouter>,
    );
    expect(
      screen.getByRole("switch", { name: "Enable notification" }),
    ).toBeInTheDocument();
    expect(
      screen
        .getAllByRole("button", { name: "Save changes" })
        .every((button) => button.hasAttribute("disabled")),
    ).toBe(true);
    expect(requestSave).not.toHaveBeenCalled();
  });
});
