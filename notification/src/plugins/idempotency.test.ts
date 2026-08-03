import { test } from "node:test";
import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import Fastify from "fastify";
import type { IdempotencyKey } from "@prisma/client";
import type { IdempotencyRepository } from "./idempotency.js";
import { hashRequestBody, idempotencyPreHandler, registerIdempotency } from "./idempotency.js";
import { statusForError } from "../lib/errors.js";

class FakeIdempotencyRepository implements IdempotencyRepository {
  records = new Map<string, IdempotencyKey>();

  async reserve(accountId: string, key: string, requestHash: string) {
    const mapKey = `${accountId}|${key}`;
    const existing = this.records.get(mapKey);
    if (existing) return { record: existing, reserved: false };

    const record: IdempotencyKey = {
      uuid: randomUUID(),
      accountId,
      key,
      requestHash,
      responseStatus: null,
      responseBody: null,
      createdAt: new Date(),
    };
    this.records.set(mapKey, record);
    return { record, reserved: true };
  }

  async complete(id: string, responseStatus: number, responseBody: Buffer) {
    for (const record of this.records.values()) {
      if (record.uuid === id) {
        record.responseStatus = responseStatus;
        record.responseBody = responseBody;
        return;
      }
    }
  }
}

function buildTestApp(repo: IdempotencyRepository, calls: { count: number }) {
  const app = Fastify();
  app.setErrorHandler((err: Error, _request, reply) => {
    reply.code(statusForError(err) ?? 500).send({ error: err.message });
  });

  registerIdempotency(app, repo);

  app.decorateRequest("account", null);
  app.addHook("onRequest", async (request) => {
    request.account = { uuid: "acc-1" } as never;
  });
  app.post(
    "/",
    { preHandler: idempotencyPreHandler(repo) },
    async (_request, reply) => {
      calls.count++;
      return reply.code(201).send({ ok: true });
    },
  );

  return app;
}

test("no key passes through and always runs the handler", async () => {
  const calls = { count: 0 };
  const app = buildTestApp(new FakeIdempotencyRepository(), calls);

  const res = await app.inject({ method: "POST", url: "/", payload: {} });
  assert.equal(res.statusCode, 201);
  assert.equal(calls.count, 1);
});

test("first request with a key runs the handler and persists the response", async () => {
  const calls = { count: 0 };
  const repo = new FakeIdempotencyRepository();
  const app = buildTestApp(repo, calls);

  const res = await app.inject({
    method: "POST",
    url: "/",
    payload: { a: 1 },
    headers: { "idempotency-key": "key-1" },
  });

  assert.equal(res.statusCode, 201);
  assert.equal(calls.count, 1);

  const stored = repo.records.get("acc-1|key-1");
  assert.equal(stored?.responseStatus, 201);
});

test("repeated key with the same body replays without rerunning the handler", async () => {
  const calls = { count: 0 };
  const repo = new FakeIdempotencyRepository();
  const app = buildTestApp(repo, calls);

  const payload = { a: 1 };
  const first = await app.inject({ method: "POST", url: "/", payload, headers: { "idempotency-key": "key-2" } });
  const second = await app.inject({ method: "POST", url: "/", payload, headers: { "idempotency-key": "key-2" } });

  assert.equal(calls.count, 1);
  assert.equal(second.statusCode, 201);
  assert.deepEqual(second.json(), first.json());
});

test("repeated key with a different body returns 422", async () => {
  const calls = { count: 0 };
  const repo = new FakeIdempotencyRepository();
  const app = buildTestApp(repo, calls);

  await app.inject({ method: "POST", url: "/", payload: { a: 1 }, headers: { "idempotency-key": "key-3" } });
  const second = await app.inject({
    method: "POST",
    url: "/",
    payload: { a: 2 },
    headers: { "idempotency-key": "key-3" },
  });

  assert.equal(calls.count, 1);
  assert.equal(second.statusCode, 422);
});

test("an in-flight duplicate returns 409", async () => {
  const calls = { count: 0 };
  const repo = new FakeIdempotencyRepository();
  // Simulate a first request that's still being processed: reserved but not yet completed.
  repo.records.set("acc-1|key-4", {
    uuid: randomUUID(),
    accountId: "acc-1",
    key: "key-4",
    requestHash: hashRequestBody({ a: 1 }),
    responseStatus: null,
    responseBody: null,
    createdAt: new Date(),
  });
  const app = buildTestApp(repo, calls);

  const res = await app.inject({
    method: "POST",
    url: "/",
    payload: { a: 1 },
    headers: { "idempotency-key": "key-4" },
  });

  assert.equal(calls.count, 0);
  assert.equal(res.statusCode, 409);
});

test("key order in the request body doesn't cause a false mismatch", async () => {
  const calls = { count: 0 };
  const repo = new FakeIdempotencyRepository();
  const app = buildTestApp(repo, calls);

  const first = await app.inject({
    method: "POST",
    url: "/",
    payload: { a: 1, b: 2 },
    headers: { "idempotency-key": "key-5" },
  });
  const second = await app.inject({
    method: "POST",
    url: "/",
    payload: { b: 2, a: 1 },
    headers: { "idempotency-key": "key-5" },
  });

  assert.equal(calls.count, 1);
  assert.equal(second.statusCode, 201);
  assert.deepEqual(second.json(), first.json());
});
