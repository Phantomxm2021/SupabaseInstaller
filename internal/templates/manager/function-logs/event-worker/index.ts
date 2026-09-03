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
  metadata: { service_path?: string; execution_id?: string };
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

async function normalize(data: RuntimeEvent): Promise<LogEvent | undefined> {
  const prefix = "./examples/";
  const servicePath = data.metadata?.service_path ?? "";
  const functionName = servicePath.startsWith(prefix) ? servicePath.slice(prefix.length) : "";
  const executionId = data.metadata?.execution_id ?? "";
  if (functionName === "main" || !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(functionName) || !executionId) return;

  let message = "";
  let level: LogEvent["level"];
  switch (data.event_type) {
    case "Boot":
      level = "info";
      break;
    case "Log": {
      if (typeof data.event?.msg !== "string") return;
      message = data.event.msg;
      const levels: Record<string, LogEvent["level"]> = {
        Debug: "debug", Info: "info", Warn: "warn", Warning: "warn", Error: "error",
      };
      level = levels[String(data.event.level)];
      if (!level) return;
      break;
    }
    case "UncaughtException":
      if (typeof data.event?.exception !== "string") return;
      message = data.event.exception;
      level = "error";
      break;
    default:
      return;
  }

  // JSON.stringify is the canonical parser input for object callbacks. Hash
  // exactly these UTF-8 bytes so retries of an identical callback deduplicate.
  const raw = new TextEncoder().encode(JSON.stringify(data));
  const eventId = hex(await crypto.subtle.digest("SHA-256", raw));
  return { version: 1, eventId, functionName, executionId, eventType: data.event_type, message, timestamp: data.timestamp, level };
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

export {};
