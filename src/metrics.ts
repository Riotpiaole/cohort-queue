/**
 * Observability per SOFTWARE_DEV.md: two strictly separated channels.
 *
 *  - serviceMetric     -> aggregatable KPIs (latency, counts, throughput)
 *  - applicationMetric -> runtime health / errors, always with a stack trace
 *
 * Both write to stdout/stderr in a grep-friendly, fixed format so a log shipper
 * (Datadog, OTel collector, etc.) can parse them without a structured logger.
 */

export type Severity = "INFO" | "WARN" | "ERROR";

/** Emit a measurable KPI: `[SERVICE_METRIC] <key>=<value> <unit>`. */
export function serviceMetric(key: string, value: number, unit: string): void {
  console.log(`[SERVICE_METRIC] ${key}=${value} ${unit}`);
}

/**
 * Emit a health/error diagnostic: `[APP_METRIC] <severity> <message> | trace=<stack>`.
 * Never swallow errors silently — every catch block routes through here.
 */
export function applicationMetric(
  severity: Severity,
  message: string,
  err?: unknown,
): void {
  const trace = err instanceof Error ? (err.stack ?? err.message) : err === undefined ? "" : String(err);
  const line = `[APP_METRIC] ${severity} ${message} | trace=${trace}`;
  if (severity === "ERROR") {
    console.error(line);
  } else {
    console.log(line);
  }
}
