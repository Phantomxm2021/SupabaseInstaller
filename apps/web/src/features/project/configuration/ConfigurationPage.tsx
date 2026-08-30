import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RotateCw } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import {
  Link,
  Navigate,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { toast } from "sonner";
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
import { APIError, apiFetch } from "../../../api/client";
import type {
  DatabaseConfig,
  FunctionsConfig,
  GeneralConfig,
  NetworkConfig,
  PoolerConfig,
  RedactedProjectConfiguration,
  Services,
  StorageConfig,
} from "../../../api/types";
import { OperationPanel } from "../../operations/OperationPanel";
import { DatabaseSection } from "./DatabaseSection";
import { FunctionsSection } from "./FunctionsSection";
import { GeneralSection } from "./GeneralSection";
import { NetworkSection, type TLSUpload } from "./NetworkSection";
import { PoolerSection } from "./PoolerSection";
import { RealtimeSection } from "./RealtimeSection";
import { SecretsSection } from "./SecretsSection";
import { ServicesSection } from "./ServicesSection";
import { StorageSection } from "./StorageSection";
import {
  affectedServices,
  CONFIGURATION_SECTIONS,
  dirtyLabels,
  normalizeRedactedConfiguration,
  sectionImpact,
  sectionLabel,
  SECTION_LABELS,
  type ConfigurationSection,
  type PendingConfigurationSave,
} from "./types";
import { useConfigurationMutation } from "./useConfigurationMutation";

type Snapshot = {
  projectId: string;
  revision: number;
  lastGoodRevision: number;
  configuration: RedactedProjectConfiguration;
  projectUrl?: string;
  anonKey?: string;
};
type SaveInput = {
  value: unknown;
  dirty: unknown;
  setError: (name: string, message: string) => void;
};
const projectSettingsGroups: ReadonlyArray<{
  label: string;
  items: readonly ConfigurationSection[];
}> = [
  { label: "Project", items: ["general", "secrets"] },
  {
    label: "Infrastructure",
    items: [
      "services",
      "database",
      "storage",
      "realtime",
      "functions",
      "pooler",
      "network",
    ],
  },
];

function ProjectSettingsNavigation({
  projectId,
  section,
}: {
  projectId: string;
  section: ConfigurationSection;
}) {
  return (
    <nav
      aria-label="Project settings navigation"
      className="project-settings-navigation"
    >
      <header className="project-settings-navigation-title">
        <h1>Project Settings</h1>
      </header>
      {projectSettingsGroups.map((group) => (
        <section
          className="project-settings-navigation-group"
          key={group.label}
          aria-labelledby={`project-settings-${group.label.toLowerCase()}`}
        >
          <h2
            id={`project-settings-${group.label.toLowerCase()}`}
            className="project-settings-navigation-label"
          >
            {group.label.toUpperCase()}
          </h2>
          {group.items.length > 0 && (
            <ul className="project-settings-navigation-list">
              {group.items.map((item) => (
                <li key={item}>
                  <Link
                    to={`/projects/${projectId}/configuration${item === "general" ? "" : `?section=${item}`}`}
                    className="project-settings-navigation-link"
                    aria-current={section === item ? "page" : undefined}
                  >
                    {SECTION_LABELS[item]}
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </section>
      ))}
    </nav>
  );
}

export function ConfigurationPage() {
  const { projectId = "" } = useParams();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const requested = params.get("section") ?? "general";
  const authenticationRedirect =
    requested === "smtp"
      ? "emails"
      : requested === "auth" || requested === "oauth"
        ? "sign-in-providers"
        : undefined;
  const section: ConfigurationSection = CONFIGURATION_SECTIONS.includes(
    requested as ConfigurationSection,
  )
    ? (requested as ConfigurationSection)
    : "general";
  const configuration = useQuery({
    queryKey: ["project-configuration", projectId],
    queryFn: () =>
      apiFetch<Snapshot>(`/api/projects/${projectId}/configuration`),
    enabled: Boolean(projectId) && !authenticationRedirect,
  });
  const [pending, setPending] = useState<PendingConfigurationSave>();
  const [operation, setOperation] = useState<{
    projectId: string;
    operationId: string;
  }>();
  const [conflict, setConflict] = useState(false);
  const [rotateOpen, setRotateOpen] = useState(false);
  const [rotatePassword, setRotatePassword] = useState("");
  const [reloadNonce, setReloadNonce] = useState(0);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const config = useMemo(
    () =>
      configuration.data
        ? normalizeRedactedConfiguration(configuration.data.configuration)
        : undefined,
    [configuration.data?.configuration, configuration.data?.revision],
  );
  const handleConfigurationQueued = (result: {
    projectId: string;
    operationId: string;
  }) => {
    setPending(undefined);
    setOperation({
      projectId: result.projectId,
      operationId: result.operationId,
    });
    setConflict(false);
    setFieldErrors({});
  };
  const handleConfigurationError = (error: Error) => {
    if (error instanceof APIError && error.status === 409) {
      setConflict(true);
      setPending(undefined);
    }
    if (error instanceof APIError && error.fields) setFieldErrors(error.fields);
    toast.error(error.message);
  };
  const update = useConfigurationMutation(
    projectId,
    configuration.data?.revision ?? 0,
    (result) => {
      handleConfigurationQueued(result);
      toast.success("Configuration update queued");
    },
    handleConfigurationError,
  );
  const uploadTLS = useMutation({
    mutationFn: (input: TLSUpload) => {
      const body = new FormData();
      body.set("certificateName", input.certificateName);
      if (input.certificate) body.set("certificate", input.certificate);
      if (input.privateKey) body.set("privateKey", input.privateKey);
      return apiFetch<{ projectId: string; operationId: string }>(
        `/api/projects/${projectId}/configuration/network/tls`,
        { method: "PATCH", body },
      );
    },
    onSuccess: (result) => {
      handleConfigurationQueued(result);
      toast.success("TLS certificate update queued");
    },
    onError: (error) =>
      handleConfigurationError(
        error instanceof Error ? error : new Error("TLS certificate upload failed"),
      ),
  });
  const rotate = useMutation({
    mutationFn: () =>
      apiFetch<{ projectId: string; operationId: string }>(
        `/api/projects/${projectId}/secrets/databasePassword/rotate`,
        { method: "POST", body: JSON.stringify({ password: rotatePassword }) },
      ),
    onSuccess: (result) => {
      setRotateOpen(false);
      setRotatePassword("");
      setOperation(result);
      toast.success("Database password rotation queued");
    },
    onError: (error: Error) => toast.error(error.message),
  });
  useEffect(() => {
    if (!authenticationRedirect && requested !== section)
      navigate(`/projects/${projectId}/configuration?section=${section}`, {
        replace: true,
      });
  }, [authenticationRedirect, navigate, projectId, requested, section]);
  const submit = ({ value, dirty, setError }: SaveInput) => {
    const labels = dirtyLabels(dirty).map((name) =>
      name.replaceAll(".", " → "),
    );
    if (!labels.length || !config) return;
    const services = affectedServices(section, dirty, value, config.services);
    const impact =
      section === "services" && value && typeof value === "object"
        ? serviceImpact(dirty, value, config.services)
        : sectionImpact(section, value, config.services);
    setPending({ section, value, labels, services, impact, setError });
  };
  const save = (input: SaveInput) => submit(input);
  const reload = async () => {
    setConflict(false);
    setFieldErrors({});
    setPending(undefined);
    await configuration.refetch();
    setReloadNonce((value) => value + 1);
  };
  const completed = async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: ["project-configuration", projectId],
      }),
      queryClient.invalidateQueries({ queryKey: ["project", projectId] }),
    ]);
    setOperation(undefined);
  };
  if (authenticationRedirect)
    return (
      <Navigate
        to={`/projects/${projectId}/authentication/${authenticationRedirect}`}
        replace
      />
    );
  if (configuration.isLoading)
    return (
      <main className="page">
        <div className="empty-state">Loading configuration…</div>
      </main>
    );
  if (configuration.error || !configuration.data || !config)
    return (
      <main className="page">
        <Alert variant="destructive">
          Unable to load project configuration.
        </Alert>
      </main>
    );
  if (operation)
    return (
      <main className="page configuration-page">
        <div className="page-heading">
          <div>
            <p className="eyebrow">Configuration operation</p>
            <h1>Applying project configuration</h1>
            <p className="muted">
              The runtime is reconciled in the background. This panel remains
              available until completion.
            </p>
          </div>
        </div>
        <OperationPanel
          operationId={operation.operationId}
          projectId={projectId}
          projectName={projectId}
          onSucceeded={completed}
        />
      </main>
    );
  return (
    <section className="project-settings-workspace" data-density="dashboard">
      <ProjectSettingsNavigation projectId={projectId} section={section} />
      <div className="project-settings-content">
        <main className="page configuration-page">
          <div className="page-heading">
            <div>
              <p className="eyebrow">Installed project</p>
              <h1>{SECTION_LABELS[section]}</h1>
              <p className="muted">
                Typed settings for this Supabase host. Secrets remain encrypted
                and redacted.
              </p>
            </div>
            <Badge variant="outline">
              Revision {configuration.data.revision}
            </Badge>
          </div>
          {conflict && (
            <Alert variant="destructive" className="mb-4">
              <div className="flex items-center justify-between gap-3">
                <span>
                  This configuration is stale. Your dirty fields are preserved.
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void reload()}
                >
                  Reload
                </Button>
              </div>
            </Alert>
          )}
          {Object.keys(fieldErrors).length > 0 && (
            <Alert variant="destructive" className="mb-4">
              <strong>Some fields need attention</strong>
              <ul className="mt-1 list-disc pl-5">
                {Object.entries(fieldErrors).map(([field, message]) => (
                  <li key={field}>
                    {field.replace(/^(auth\.|storage\.|functions\.)/, "")}:{" "}
                    {message}
                  </li>
                ))}
              </ul>
            </Alert>
          )}
          <div key={`${section}-${reloadNonce}`}>
            {section === "general" && (
              <GeneralSection
                revision={configuration.data.revision}
                initial={config.general}
                onSave={save}
                serverErrors={fieldErrors}
              />
            )}
            {section === "services" && (
              <ServicesSection
                revision={configuration.data.revision}
                initial={config.services}
                onSave={save}
              />
            )}
            {section === "storage" && (
              <StorageSection
                revision={configuration.data.revision}
                initial={config.storage as StorageConfig}
                storageEnabled={config.services.storage}
                onSave={save}
              />
            )}
            {section === "realtime" && (
              <RealtimeSection
                revision={configuration.data.revision}
                initial={config.realtime}
                enabled={config.services.realtime}
                onSave={save}
              />
            )}
            {section === "functions" && (
              <FunctionsSection
                revision={configuration.data.revision}
                initial={config.functions as FunctionsConfig}
                enabled={config.services.functions}
                onSave={save}
              />
            )}
            {section === "database" && (
              <DatabaseSection
                revision={configuration.data.revision}
                initial={config.database as DatabaseConfig}
                directDb={config.services.directDb}
                onSave={save}
                onRotate={() => setRotateOpen(true)}
              />
            )}
            {section === "pooler" && (
              <PoolerSection
                revision={configuration.data.revision}
                initial={config.pooler as PoolerConfig}
                enabled={config.services.supavisor}
                onSave={save}
              />
            )}
            {section === "network" && (
              <NetworkSection
                revision={configuration.data.revision}
                initial={config.network}
                siteURL={config.general.siteUrl}
                onSave={save}
                onUploadTLS={(input) => uploadTLS.mutate(input)}
                tlsUploading={uploadTLS.isPending}
              />
            )}
            {section === "secrets" && (
              <SecretsSection
                projectId={projectId}
                projectUrl={
                  configuration.data.projectUrl ??
                  (config.general.domain
                    ? `https://${config.general.domain}`
                    : "")
                }
                anonKey={configuration.data.anonKey ?? ""}
              />
            )}
          </div>
          <AlertDialog
            open={Boolean(pending)}
            onOpenChange={(open) => !open && setPending(undefined)}
          >
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  Apply configuration changes?
                </AlertDialogTitle>
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
                      {pending.services.join(", ") ||
                        "Configuration metadata only"}
                    </p>
                  </div>
                  <Badge variant="outline">
                    {pending.impact === "recreate"
                      ? "Runtime recreate required"
                      : pending.impact === "restart"
                        ? "Service restart required"
                        : pending.impact === "start"
                          ? "Service will be started"
                          : pending.impact === "stop"
                            ? "Service will be stopped"
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
          <AlertDialog open={rotateOpen} onOpenChange={setRotateOpen}>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Rotate database password?</AlertDialogTitle>
                <AlertDialogDescription>
                  Existing database clients will need the new password after
                  reconciliation. This action is audited and cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <label
                htmlFor="rotate-admin-password"
                className="text-sm font-medium"
              >
                Administrator password
              </label>
              <Input
                id="rotate-admin-password"
                type="password"
                autoComplete="current-password"
                value={rotatePassword}
                onChange={(event) => setRotatePassword(event.target.value)}
              />
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  disabled={!rotatePassword || rotate.isPending}
                  onClick={() => rotate.mutate()}
                >
                  <RotateCw className="size-4" />
                  Confirm rotation
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </main>
      </div>
    </section>
  );
}

function serviceImpact(
  _dirty: unknown,
  value: unknown,
  baseline: Services,
): PendingConfigurationSave["impact"] {
  if (
    !_dirty ||
    typeof _dirty !== "object" ||
    !value ||
    typeof value !== "object"
  )
    return "none";
  const current = value as Services;
  const changed = (Object.keys(current) as Array<keyof Services>).filter(
    (key) => baseline[key] !== current[key],
  );
  if (!changed.length) return "none";
  if (changed.some((key) => baseline[key] && !current[key])) return "stop";
  if (changed.some((key) => !baseline[key] && current[key])) return "start";
  return "recreate";
}
