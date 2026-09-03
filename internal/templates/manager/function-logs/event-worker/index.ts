// Edge Runtime v1.74.0 event workers consume an async iterator exposed as
// globalThis.EventManager. Keep this file dependency-free: the runtime event
// worker has the built-in Web and Deno APIs but no user import map.
const MAX_QUEUE = 1000;
const MAX_BATCH = 100;
const FLUSH_INTERVAL_MS = 250;
const FETCH_TIMEOUT_MS = 500;
const COLLECTOR_URL = "http://function-log-collector:8081/internal/v1/events";

type RuntimeEvent = {
  timestamp: string;
  event_type: string;
  event: Record<string, unknown>;
  metadata: {
    service_path?: string;
    execution_id?: string;
    otel_attributes?: Record<string, string> | null;
  };
};

type CanonicalRuntimeEvent = {
  timestamp: string;
  event_type: string;
  event: Record<string, unknown>;
  metadata: {
    service_path?: string;
    execution_id?: string;
    otel_attributes: Record<string, string> | null;
  };
};

type LogEvent = {
  version: 1;
  eventId: string;
  functionName: string;
  executionId: string;
  eventType: string;
  message: string;
  timestamp: string;
  level: "debug" | "info" | "warn" | "error";
};

declare global {
  // Supabase installs this constructor only in the dedicated event worker.
  // deno-lint-ignore no-var
  var EventManager: new () => AsyncIterable<RuntimeEvent | undefined>;
}

const projectId = Deno.env.get("FUNCTION_LOG_PROJECT_ID") ?? "";
const queue: LogEvent[] = [];
let flushing = false;

function hex(bytes: ArrayBuffer): string {
  return Array.from(new Uint8Array(bytes), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function canonicalRuntimeEvent(data: RuntimeEvent): CanonicalRuntimeEvent | undefined {
  let event: Record<string, unknown>;
  switch (data.event_type) {
    case "Boot":
      if (typeof data.event?.boot_time !== "number") return;
      event = { boot_time: data.event.boot_time };
      break;
    case "Log":
      if (typeof data.event?.msg !== "string" || typeof data.event?.level !== "string") return;
      event = { msg: data.event.msg, level: data.event.level };
      break;
    case "UncaughtException":
      if (typeof data.event?.exception !== "string" || typeof data.event?.cpu_time_used !== "number") return;
      event = { exception: data.event.exception, cpu_time_used: data.event.cpu_time_used };
      break;
    default:
      return;
  }
  const attributes = data.metadata?.otel_attributes;
  const sortedAttributes = attributes == null
    ? null
    : Object.fromEntries(Object.keys(attributes).sort().map((key) => [key, attributes[key]]));
  return {
    timestamp: data.timestamp,
    event_type: data.event_type,
    event,
    metadata: {
      service_path: data.metadata?.service_path,
      execution_id: data.metadata?.execution_id,
      otel_attributes: sortedAttributes,
    },
  };
}

async function normalize(data: RuntimeEvent): Promise<LogEvent | undefined> {
  const canonical = canonicalRuntimeEvent(data);
  if (!canonical) return;
  const prefix = "./examples/";
  const servicePath = canonical.metadata.service_path ?? "";
  const functionName = servicePath.startsWith(prefix) ? servicePath.slice(prefix.length) : "";
  const executionId = canonical.metadata.execution_id ?? "";
  if (functionName === "main" || !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(functionName) || !executionId) return;

  let message = "";
  let level: LogEvent["level"];
  switch (canonical.event_type) {
    case "Boot":
      level = "info";
      break;
    case "Log": {
      message = String(canonical.event.msg);
      const levels: Record<string, LogEvent["level"]> = {
        Debug: "debug", Info: "info", Warn: "warn", Warning: "warn", Error: "error",
      };
      level = levels[String(canonical.event.level)];
      if (!level) return;
      break;
    }
    case "UncaughtException":
      message = String(canonical.event.exception);
      level = "error";
      break;
    default:
      return;
  }

  // Hash the fixed-order callback object shared with the Go canonicalizer.
  const raw = new TextEncoder().encode(JSON.stringify(canonical));
  const eventId = hex(await crypto.subtle.digest("SHA-256", raw));
  return { version: 1, eventId, functionName, executionId, eventType: canonical.event_type, message, timestamp: canonical.timestamp, level };
}

function enqueue(event: LogEvent): void {
  if (queue.length === MAX_QUEUE) queue.shift();
  queue.push(event);
  if (queue.length >= MAX_BATCH) void flush();
}

async function flush(): Promise<void> {
  if (flushing || queue.length === 0 || !projectId) return;
  flushing = true;
  const events = queue.splice(0, MAX_BATCH);
  const signal = AbortSignal.timeout(FETCH_TIMEOUT_MS);
  try {
    const response = await fetch(COLLECTOR_URL, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ version: 1, projectId, events }),
      signal,
    });
    if (!response.ok) throw new Error(`collector status ${response.status}`);
  } catch {
    // Collection is best-effort and must never affect user function execution.
  } finally {
    flushing = false;
    if (queue.length >= MAX_BATCH) void flush();
  }
}

const verificationFixtures = Deno.env.get("FUNCTION_LOG_VERIFY_FIXTURES");
if (verificationFixtures) {
  const fixtures = await Promise.all(
    ["boot-event.json", "log-event.json", "uncaught-exception.json"].map(async (name) =>
      JSON.parse(await Deno.readTextFile(`${verificationFixtures}/${name}`)) as RuntimeEvent
    ),
  );
  const records: LogEvent[] = [];
  for (const fixture of fixtures) {
    const record = await normalize(fixture);
    if (!record) throw new Error("fixture normalization failed");
    records.push(record);
  }
  console.log("FUNCTION_LOG_FIXTURE_RECORDS=" + JSON.stringify(records));
} else {
  setInterval(() => void flush(), FLUSH_INTERVAL_MS);
  const eventManager = new globalThis.EventManager();
  for await (const data of eventManager) {
    try {
      if (data) {
        const event = await normalize(data);
        if (event) enqueue(event);
      }
    } catch {
      // Malformed callbacks and collector failures never escape into the runtime.
    }
  }
  await flush();
}

export {};
