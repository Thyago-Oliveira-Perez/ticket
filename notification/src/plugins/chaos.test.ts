import { test } from "node:test";
import assert from "node:assert/strict";
import Fastify from "fastify";
import { registerChaos } from "./chaos.js";
import type { ChaosConfig } from "../config.js";

const noChaos: ChaosConfig = {
  latencyMinMs: 0,
  latencyMaxMs: 0,
  errorRate: 0,
  rateLimitRps: 0,
  rateLimitBurst: 0,
};

function buildTestApp(cfg: Partial<ChaosConfig>) {
  const app = Fastify();
  registerChaos(app, { ...noChaos, ...cfg });
  app.get("/", async () => ({ ok: true }));
  return app;
}

test("latency disabled adds no delay", async () => {
  const app = buildTestApp({});
  const start = Date.now();
  const res = await app.inject({ method: "GET", url: "/" });
  assert.equal(res.statusCode, 200);
  assert.ok(Date.now() - start < 50, "expected no delay");
});

test("latency injects a delay within bounds", async () => {
  const app = buildTestApp({ latencyMinMs: 20, latencyMaxMs: 40 });
  const start = Date.now();
  const res = await app.inject({ method: "GET", url: "/" });
  const elapsed = Date.now() - start;
  assert.equal(res.statusCode, 200);
  assert.ok(elapsed >= 20, `expected delay >= 20ms, took ${elapsed}ms`);
  assert.ok(elapsed <= 40 + 50, `expected delay close to <= 40ms, took ${elapsed}ms`);
});

test("random error disabled always succeeds", async () => {
  const app = buildTestApp({});
  const res = await app.inject({ method: "GET", url: "/" });
  assert.equal(res.statusCode, 200);
});

test("random error at rate 1 always fails", async () => {
  const app = buildTestApp({ errorRate: 1 });
  const res = await app.inject({ method: "GET", url: "/" });
  assert.equal(res.statusCode, 500);
});

test("rate limit disabled never blocks", async () => {
  const app = buildTestApp({});
  for (let i = 0; i < 10; i++) {
    const res = await app.inject({ method: "GET", url: "/", remoteAddress: "1.2.3.4" });
    assert.equal(res.statusCode, 200);
  }
});

test("rate limit blocks after burst is exhausted", async () => {
  const app = buildTestApp({ rateLimitRps: 1, rateLimitBurst: 1 });

  const first = await app.inject({ method: "GET", url: "/", remoteAddress: "1.2.3.4" });
  assert.equal(first.statusCode, 200);

  const second = await app.inject({ method: "GET", url: "/", remoteAddress: "1.2.3.4" });
  assert.equal(second.statusCode, 429);
});

test("rate limit isolates buckets per client IP", async () => {
  const app = buildTestApp({ rateLimitRps: 1, rateLimitBurst: 1 });

  const a = await app.inject({ method: "GET", url: "/", remoteAddress: "1.2.3.4" });
  const b = await app.inject({ method: "GET", url: "/", remoteAddress: "5.6.7.8" });

  assert.equal(a.statusCode, 200);
  assert.equal(b.statusCode, 200);
});
