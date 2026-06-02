/**
 * Express server for the cohort waiting-list frontend.
 *
 * Serves the visualization from /public and reverse-proxies every /api/* request
 * to the Go coordinator (COORDINATOR_URL). This keeps the browser same-origin (no
 * CORS) while the real queue lives in the coordinator. Each proxied call is timed
 * (serviceMetric) and surfaces upstream failures (applicationMetric) — no silent
 * catches (SOFTWARE_DEV.md).
 */

import { fileURLToPath } from "node:url";
import path from "node:path";
import express, { type Request, type Response } from "express";

import { serviceMetric, applicationMetric } from "./metrics.js";
import type { ApiError } from "./types.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PUBLIC_DIR = path.resolve(__dirname, "../public");
const PORT = Number(process.env.PORT) || 3000;
const COORDINATOR_URL = process.env.COORDINATOR_URL ?? "http://localhost:8080";

const app = express();
app.use(express.json());
app.use(express.static(PUBLIC_DIR));

/**
 * Reverse-proxy /api/* to the coordinator. Forwards method, path, and JSON body;
 * relays the upstream status + body back to the browser. A short op label (e.g.
 * "add") is derived from the path for the latency metric.
 */
app.all("/api/*", async (req: Request, res: Response) => {
  const op = req.path.replace(/^\/api\//, "") || "root";
  const startedAtMs = performance.now();
  try {
    const init: RequestInit = {
      method: req.method,
      headers: { "content-type": "application/json" },
    };
    // Only forward a body for methods that have one.
    if (req.method !== "GET" && req.method !== "HEAD") {
      init.body = JSON.stringify(req.body ?? {});
    }

    const upstream = await fetch(`${COORDINATOR_URL}${req.originalUrl}`, init);
    const text = await upstream.text();
    res
      .status(upstream.status)
      .type(upstream.headers.get("content-type") ?? "application/json")
      .send(text);
  } catch (err) {
    // Coordinator unreachable (down, or port-forward not running).
    applicationMetric("ERROR", `api.${op} proxy to ${COORDINATOR_URL} failed`, err);
    if (!res.headersSent) {
      const body: ApiError = { error: `Coordinator unreachable at ${COORDINATOR_URL}.` };
      res.status(502).json(body);
    }
  } finally {
    const latencyMs = Math.round(performance.now() - startedAtMs);
    serviceMetric(`api.proxy.${op}.latency_ms`, latencyMs, "ms");
  }
});

app.listen(PORT, () => {
  applicationMetric(
    "INFO",
    `cohort frontend on http://localhost:${PORT} -> coordinator ${COORDINATOR_URL}`,
  );
});
