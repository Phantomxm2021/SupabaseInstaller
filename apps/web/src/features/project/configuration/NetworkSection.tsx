import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm, type Resolver } from "react-hook-form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import type { NetworkConfig } from "../../../api/types";
import { networkSchema } from "./schema";
import {
  SectionCard,
  ReadOnlyField,
  SectionSaveButton,
  useResetOnServerRevision,
  errorAt,
  type SectionSave,
} from "./fields";

export type TLSUpload = {
  certificateName: string;
  certificate: File;
  privateKey: File;
};

export function NetworkSection({
  initial,
  revision,
  onSave,
  onUploadTLS,
  tlsUploading = false,
}: {
  initial: NetworkConfig;
  revision: number;
  onSave: SectionSave<NetworkConfig>;
  onUploadTLS: (input: TLSUpload) => void;
  tlsUploading?: boolean;
}) {
  const form = useForm<NetworkConfig>({
    resolver: zodResolver(networkSchema) as Resolver<NetworkConfig>,
    defaultValues: initial,
  });
  useResetOnServerRevision(form, initial, revision);
  const network = form.watch();
  const setError = (name: string, message: string) =>
    form.setError(name as never, { type: "server", message });
  const gatewayError = errorAt(form.formState.errors, "gateway");
  const httpsError = errorAt(form.formState.errors, "httpsMode");
  const managedTLS = initial.managedTls;
  const [certificateName, setCertificateName] = useState(
    managedTLS?.certificateName ?? "cloudflare-origin",
  );
  const [certificate, setCertificate] = useState<File>();
  const [privateKey, setPrivateKey] = useState<File>();

  return (
    <div className="space-y-5">
      <form
        id="configuration-network-form"
        onSubmit={form.handleSubmit((value) =>
          onSave({ value, dirty: form.formState.dirtyFields, setError }),
        )}
        className="space-y-5"
      >
        <SectionCard
          title="Gateway & Network"
          description="Gateway selection and HTTPS mode are typed. Manager-allocated ports are read-only."
        >
          <div className="space-y-4">
            <div>
              <label htmlFor="network-gateway" className="text-sm font-medium">
                Gateway
              </label>
              <Select
                value={network.gateway}
                onValueChange={(value) =>
                  form.setValue("gateway", value as NetworkConfig["gateway"], {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                }
              >
                <SelectTrigger
                  id="network-gateway"
                  aria-label="Gateway"
                  aria-invalid={Boolean(gatewayError)}
                  aria-describedby={gatewayError ? "network-gateway-error" : undefined}
                  className="mt-1 w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="envoy">Envoy (recommended)</SelectItem>
                  <SelectItem value="kong">Kong (advanced)</SelectItem>
                </SelectContent>
              </Select>
              {gatewayError && <p id="network-gateway-error" className="text-sm text-destructive">{gatewayError}</p>}
            </div>
            <div>
              <label htmlFor="network-https-mode" className="text-sm font-medium">
                HTTPS mode
              </label>
              <Select
                value={network.httpsMode}
                onValueChange={(value) =>
                  form.setValue("httpsMode", value as NetworkConfig["httpsMode"], {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                }
              >
                <SelectTrigger
                  id="network-https-mode"
                  aria-label="HTTPS mode"
                  aria-invalid={Boolean(httpsError)}
                  aria-describedby={httpsError ? "network-https-mode-error" : undefined}
                  className="mt-1 w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="external">External reverse proxy</SelectItem>
                  <SelectItem value="caddy">Caddy managed</SelectItem>
                </SelectContent>
              </Select>
              {httpsError && <p id="network-https-mode-error" className="text-sm text-destructive">{httpsError}</p>}
            </div>
            <ReadOnlyField label="API port" value={String(network.apiPort || "Manager allocated")} />
            <ReadOnlyField label="Studio port" value={String(network.studioPort || "Manager allocated")} />
            <ReadOnlyField label="Direct database port" value={String(network.directDatabasePort || "Disabled")} />
            <ReadOnlyField label="Pooler port" value={String(network.poolerPort || "Manager allocated")} />
          </div>
        </SectionCard>
        <SectionSaveButton label="Gateway & Network" disabled={!form.formState.isDirty} />
      </form>

      <SectionCard
        title="TLS certificate"
        description="Replace the certificate served by the managed external Nginx site. The private key is transmitted directly to the host and is never returned in configuration reads."
      >
        <div className="space-y-5">
          <Field>
            <FieldLabel htmlFor="tls-certificate-name">Certificate name</FieldLabel>
            <Input
              id="tls-certificate-name"
              aria-label="Certificate name"
              value={certificateName}
              onChange={(event) => setCertificateName(event.target.value)}
              pattern="[a-z0-9-]+"
              required
            />
            <FieldDescription>
              Use lowercase letters, digits, and hyphens. The host writes a domain-scoped certificate file from this name.
            </FieldDescription>
          </Field>
          <div className="grid gap-5 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="settings-tls-certificate">Certificate (.pem or .crt)</FieldLabel>
              <Input
                id="settings-tls-certificate"
                type="file"
                accept=".pem,.crt,application/x-pem-file"
                required
                onChange={(event) => setCertificate(event.target.files?.[0])}
              />
              <FieldDescription>PEM certificate or certificate chain.</FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="settings-tls-private-key">Private key (.key or .pem)</FieldLabel>
              <Input
                id="settings-tls-private-key"
                type="file"
                accept=".key,.pem,application/x-pem-file"
                required
                onChange={(event) => setPrivateKey(event.target.files?.[0])}
              />
              <FieldDescription>Matching unencrypted PEM private key.</FieldDescription>
            </Field>
          </div>
          {managedTLS ? (
            <p className="text-sm text-muted-foreground">
              Active managed certificate: <code>{managedTLS.certificateName}</code>
            </p>
          ) : (
            <p className="text-sm text-muted-foreground">
              No project certificate is configured. The host default is used until you upload a replacement.
            </p>
          )}
          <div className="flex justify-end">
            <Button
              type="button"
              disabled={tlsUploading || network.httpsMode !== "external" || !certificate || !privateKey}
              onClick={() => {
                if (!certificate || !privateKey) return;
                onUploadTLS({ certificateName, certificate, privateKey });
              }}
            >
              Upload TLS certificate
            </Button>
          </div>
        </div>
      </SectionCard>
    </div>
  );
}
