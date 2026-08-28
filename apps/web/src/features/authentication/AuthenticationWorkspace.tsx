import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  Navigate,
  Outlet,
  useOutletContext,
  useParams,
} from "react-router-dom";
import { toast } from "sonner";
import { BookOpen } from "lucide-react";
import { Alert } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiFetch } from "../../api/client";
import type {
  AuthConfig,
  GeneralConfig,
  RedactedProjectConfiguration,
  Services,
} from "../../api/types";
import {
  affectedServices,
  normalizeRedactedConfiguration,
  sectionImpact,
  sectionLabel,
  type PendingConfigurationSave,
} from "../project/configuration/types";
import { useConfigurationMutation } from "../project/configuration/useConfigurationMutation";
import { OperationPanel } from "../operations/OperationPanel";
import { AuthenticationNavigation } from "./navigation";
import { SignInProvidersPage } from "./SignInProvidersPage";
import { EmailsPage } from "./EmailsPage";
import { EmailTemplateEditorPage } from "./EmailTemplateEditorPage";
import { MultiFactorPage } from "./MultiFactorPage";
import { OAuthAppsPage } from "./OAuthAppsPage";
import { RateLimitsPage } from "./RateLimitsPage";
import { UsersPage } from "./UsersPage";

type Snapshot = {
  projectId: string;
  revision: number;
  configuration: RedactedProjectConfiguration;
};
type SaveRequest = Omit<
  PendingConfigurationSave,
  "labels" | "services" | "impact"
> & { dirty: unknown; onQueued?: () => void };

export type AuthenticationWorkspaceContext = {
  projectId: string;
  revision: number;
  auth: AuthConfig;
  general: GeneralConfig;
  services: Services;
  requestSave: (request: SaveRequest) => void;
};
export function useAuthenticationWorkspace() {
  return useOutletContext<AuthenticationWorkspaceContext>();
}

export function AuthenticationWorkspace() {
  const { projectId = "" } = useParams();
  const queryClient = useQueryClient();
  const configuration = useQuery({
    queryKey: ["project-configuration", projectId],
    queryFn: () =>
      apiFetch<Snapshot>(`/api/projects/${projectId}/configuration`),
    enabled: Boolean(projectId),
  });
  const [pending, setPending] = useState<
    PendingConfigurationSave & { onQueued?: () => void }
  >();
  const [operation, setOperation] = useState<{
    projectId: string;
    operationId: string;
  }>();
  const normalized = configuration.data
    ? normalizeRedactedConfiguration(configuration.data.configuration)
    : undefined;
  const update = useConfigurationMutation(
    projectId,
    configuration.data?.revision ?? 0,
    (result) => {
      const next = pending;
      setPending(undefined);
      setOperation({ projectId: result.projectId, operationId: result.operationId });
      next?.onQueued?.();
      toast.success("Configuration update queued");
    },
    (error) => toast.error(error.message),
  );
  const requestSave = (request: SaveRequest) => {
    if (!normalized) return;
    const labels = dirtyLabels(request.dirty).map((label) =>
      label.replaceAll(".", " → "),
    );
    if (!labels.length) return;
    setPending({
      ...request,
      labels,
      services: affectedServices(
        request.section,
        request.dirty,
        request.value,
        normalized.services,
      ),
      impact: sectionImpact(
        request.section,
        request.value,
        normalized.services,
      ),
    });
  };
  if (configuration.isLoading)
    return (
      <main className="page">
        <div className="empty-state">Loading configuration…</div>
      </main>
    );
  if (configuration.error || !configuration.data || !normalized)
    return (
      <main className="page">
        <Alert variant="destructive">
          Unable to load project configuration.
        </Alert>
      </main>
    );
  const context: AuthenticationWorkspaceContext = {
    projectId,
    revision: configuration.data.revision,
    auth: normalized.auth as unknown as AuthConfig,
    general: normalized.general,
    services: normalized.services,
    requestSave,
  };
  const completed = async () => {
    await queryClient.invalidateQueries({
      queryKey: ["project-configuration", projectId],
    });
    await queryClient.invalidateQueries({ queryKey: ["project", projectId] });
    setOperation(undefined);
  };
  return (
    <section className="authentication-workspace">
      <AuthenticationNavigation />
      <div className="authentication-content">
        {operation ? (
          <main className="page auth-page">
            <div className="page-heading">
              <div>
                <p className="eyebrow">Configuration operation</p>
                <h1>Applying authentication configuration</h1>
                <p className="muted">
                  The runtime status and any failure details are shown here.
                </p>
              </div>
            </div>
            <OperationPanel
              operationId={operation.operationId}
              projectId={operation.projectId}
              projectName="Authentication configuration"
              onSucceeded={completed}
            />
          </main>
        ) : (
          <Outlet context={context} />
        )}
      </div>
      <AlertDialog
        open={Boolean(pending)}
        onOpenChange={(open) => !open && setPending(undefined)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Apply configuration changes?</AlertDialogTitle>
            <AlertDialogDescription>
              Only the dirty fields in{" "}
              {pending && sectionLabel(pending.section)} will be sent.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {pending && (
            <div className="space-y-3 text-sm">
              <div>
                <strong>Changed settings</strong>
                <ul className="mt-1 list-disc pl-5">
                  {pending.labels.map((label) => (
                    <li key={label}>{label}</li>
                  ))}
                </ul>
              </div>
              <div>
                <strong>Affected services</strong>
                <p className="mt-1 text-muted-foreground">
                  {pending.services.join(", ") || "Configuration metadata only"}
                </p>
              </div>
              <Badge variant="outline">
                {pending.impact === "recreate"
                  ? "Runtime recreate required"
                  : "No runtime restart expected"}
              </Badge>
            </div>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel>Keep editing</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => pending && update.mutate(pending)}
            >
              Confirm and apply
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}

function dirtyLabels(value: unknown, path: string[] = []): string[] {
  if (value === true) return path.length ? [path.join(".")] : [];
  if (!value || typeof value !== "object") return [];
  return Object.entries(value).flatMap(([key, child]) =>
    dirtyLabels(child, [...path, key]),
  );
}

export function SignInProvidersRoute() {
  return <SignInProvidersPage />;
}

export function EmailsRoute() {
  return <EmailsPage />;
}

export function EmailTemplateEditorRoute() {
  return <EmailTemplateEditorPage />;
}

export function RateLimitsRoute() {
  return <RateLimitsPage />;
}

export function MultiFactorRoute() {
  return <MultiFactorPage />;
}

export function URLConfigurationRoute() {
  const { auth, general, requestSave } = useAuthenticationWorkspace();
  const [siteUrl, setSiteUrl] = useState(general.siteUrl);
  const [redirectUrls, setRedirectUrls] = useState(auth.redirectUrls);
  const siteDirty = siteUrl !== general.siteUrl;
  const redirectsDirty =
    JSON.stringify(redirectUrls) !== JSON.stringify(auth.redirectUrls);
  return (
    <main className="page auth-page auth-reference-page auth-url-reference-page">
      <header className="page-heading">
        <div>
          <h1>URL Configuration</h1>
          <p className="muted">
            Configure site URL and redirect URLs for authentication
          </p>
        </div>
      </header>
      <section className="auth-reference-section">
        <h2>Site URL</h2>
        <form
          className="auth-settings-card auth-url-card auth-url-site-card"
          onSubmit={(event) => {
            event.preventDefault();
            requestSave({
              section: "general",
              value: { ...general, siteUrl },
              dirty: { siteUrl: true },
            });
          }}
        >
          <div className="auth-url-site-row">
            <div>
              <h3>Site URL</h3>
              <p>
                The default redirect URL used when a redirect URL is not
                specified or doesn't match one from the allow list. This value
                is also exposed as a template variable in the email templates
                section. Wildcards cannot be used here.
              </p>
            </div>
            <Input
              aria-label="Site URL"
              value={siteUrl}
              onChange={(event) => setSiteUrl(event.target.value)}
              placeholder="https://app.example.com"
            />
          </div>
          <div className="auth-reference-card-footer">
            <Button type="submit" disabled={!siteDirty}>
              Save changes
            </Button>
          </div>
        </form>
      </section>
      <section className="auth-reference-section auth-url-redirect-section">
        <header className="auth-reference-section-heading">
          <div>
            <h2>Redirect URLs</h2>
            <p>
              URLs that auth providers are permitted to redirect to post
              authentication. Wildcards are allowed, for example,
              https://*.domain.com
            </p>
          </div>
          <div className="auth-url-heading-actions">
            <a
              className="auth-docs-link"
              href="https://supabase.com/docs/guides/auth/concepts/redirect-urls"
              target="_blank"
              rel="noreferrer"
            >
              <BookOpen aria-hidden="true" />
              Docs
            </a>
            <Button
              type="button"
              onClick={() => setRedirectUrls((current) => [...current, ""])}
            >
              Add URL
            </Button>
          </div>
        </header>
        <form
          className="auth-settings-card auth-url-card auth-url-redirect-card"
          onSubmit={(event) => {
            event.preventDefault();
            requestSave({
              section: "auth",
              value: { ...auth, redirectUrls },
              dirty: { redirectUrls: true },
            });
          }}
        >
          {redirectUrls.length === 0 ? (
            <div className="auth-url-empty-state">
              <h3>No Redirect URLs</h3>
              <p>Auth providers may need a URL to redirect back to</p>
            </div>
          ) : (
            <div className="auth-url-list">
              {redirectUrls.map((value, index) => (
                <div className="auth-url-list-row" key={`${index}-${value}`}>
                  <Input
                    aria-label={`Redirect URL ${index + 1}`}
                    value={value}
                    onChange={(event) =>
                      setRedirectUrls((current) =>
                        current.map((item, position) =>
                          position === index ? event.target.value : item,
                        ),
                      )
                    }
                    placeholder="https://app.example.com/auth/callback"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() =>
                      setRedirectUrls((current) =>
                        current.filter((_, position) => position !== index),
                      )
                    }
                  >
                    Remove
                  </Button>
                </div>
              ))}
            </div>
          )}
          {redirectUrls.length > 0 && (
            <div className="auth-reference-card-footer">
              <Button type="submit" disabled={!redirectsDirty}>
                Save changes
              </Button>
            </div>
          )}
        </form>
      </section>
    </main>
  );
}
export function UsersRoute() {
  return <UsersPage />;
}
export function OAuthAppsRoute() {
  return <OAuthAppsPage />;
}

export function RetainedAuthenticationRedirect() {
  const { projectId = "" } = useParams();
  return (
    <Navigate to={`/projects/${projectId}/authentication/emails`} replace />
  );
}
