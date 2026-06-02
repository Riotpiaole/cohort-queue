/**
 * Express server for the cohort waiting-list static demo.
 *
 * Serves the visualization from /public and exposes the 4 operations as JSON
 * endpoints. Every handler is wrapped by `withMetrics`, which emits a
 * serviceMetric (latency) on success and an applicationMetric (with stack) on
 * failure — no silent catches (SOFTWARE_DEV.md).
 */

import { fileURLToPath } from "node:url";
import path from "node:path";
import express, { type Request, type Response } from "express";

import { serviceMetric, applicationMetric } from "./metrics.js";
import * as demo from "./demo-state.js";
import type { ApiError } from "./types.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PUBLIC_DIR = path.resolve(__dirname, "../public");
const PORT = Number(process.env.PORT) || 3000;

const app = express();
app.use(express.json());
app.use(express.static(PUBLIC_DIR));

/**
 * Wrap a route handler so it is timed and never throws uncaught. On success we
 * emit `api.<op>.latency_ms`; on error we emit an ERROR applicationMetric with
 * the stack and the offending request body, then return 500.
 */
function withMetrics(
  op: string,
  handler: (req: Request, res: Response) => void,
): (req: Request, res: Response) => void {
  return (req, res) => {
    const startedAtMs = performance.now();
    try {
      handler(req, res);
    } catch (err) {
      applicationMetric("ERROR", `api.${op} failed | body=${JSON.stringify(req.body)}`, err);
      if (!res.headersSent) {
        const body: ApiError = { error: "Internal error — see server logs." };
        res.status(500).json(body);
      }
    } finally {
      const latencyMs = Math.round(performance.now() - startedAtMs);
      serviceMetric(`api.${op}.latency_ms`, latencyMs, "ms");
    }
  };
}

/**
 * Validate an optional non-negative integer from the request body. Returns the
 * number, or null after emitting a WARN + 400 (the demo ignores the value, but
 * we still exercise the error path so bad input surfaces to monitoring).
 */
function readCount(req: Request, res: Response, field: string): number | null {
  const raw = (req.body as Record<string, unknown> | undefined)?.[field];
  if (raw === undefined) return 0;
  if (typeof raw !== "number" || !Number.isInteger(raw) || raw < 0) {
    applicationMetric("WARN", `bad ${field}=${JSON.stringify(raw)} (want non-negative integer)`);
    const body: ApiError = { error: `"${field}" must be a non-negative integer.` };
    res.status(400).json(body);
    return null;
  }
  return raw;
}

// POST /api/create — reset the tour. `capacity` accepted (display-only in the demo).
app.post(
  "/api/create",
  withMetrics("create", (req, res) => {
    const rawCapacity = (req.body as Record<string, unknown> | undefined)?.capacity;
    if (rawCapacity !== undefined && (typeof rawCapacity !== "number" || rawCapacity < 1)) {
      applicationMetric("WARN", `bad capacity=${JSON.stringify(rawCapacity)} (want integer >= 1)`);
      const body: ApiError = { error: '"capacity" must be an integer >= 1.' };
      res.status(400).json(body);
      return;
    }
    res.json(demo.create(typeof rawCapacity === "number" ? rawCapacity : undefined));
  }),
);

// POST /api/add — step the tour toward the grown state.
app.post(
  "/api/add",
  withMetrics("add", (req, res) => {
    if (readCount(req, res, "n") === null) return;
    res.json(demo.add());
  }),
);

// POST /api/take — step the tour toward empty (drains the right).
app.post(
  "/api/take",
  withMetrics("take", (req, res) => {
    if (readCount(req, res, "n") === null) return;
    res.json(demo.take());
  }),
);

// GET /api/total — total creators currently waiting.
app.get(
  "/api/total",
  withMetrics("total", (_req, res) => {
    res.json({ total: demo.total() });
  }),
);

app.listen(PORT, () => {
  applicationMetric("INFO", `cohort demo listening on http://localhost:${PORT}`);
});
