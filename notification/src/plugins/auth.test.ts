import { test } from "node:test";
import assert from "node:assert/strict";
import Fastify from "fastify";
import type { Account } from "@prisma/client";
import type { AccountService } from "../modules/accounts/service.js";
import { InvalidApiKeyError } from "../modules/accounts/service.js";
import { statusForError } from "../lib/errors.js";
import { registerAuth } from "./auth.js";

function buildTestApp(service: AccountService) {
  const app = Fastify();
  app.setErrorHandler((err: Error, _request, reply) => {
    reply.code(statusForError(err) ?? 500).send({ error: err.message });
  });
  registerAuth(app, service);
  app.get("/protected", { preHandler: app.authenticate }, async (request) => ({
    accountId: request.account?.uuid,
  }));
  return app;
}

const fakeAccount: Account = {
  uuid: "11111111-1111-1111-1111-111111111111",
  name: "Acme",
  status: "active",
  createdAt: new Date(),
  updatedAt: new Date(),
};

const stubService: AccountService = {
  async createAccount() {
    throw new Error("not used in this test");
  },
  async authenticateApiKey(rawKey: string) {
    if (rawKey === "good-key") return fakeAccount;
    throw new InvalidApiKeyError("invalid api key");
  },
};

test("rejects a request with no Authorization header", async () => {
  const app = buildTestApp(stubService);
  const res = await app.inject({ method: "GET", url: "/protected" });
  assert.equal(res.statusCode, 401);
});

test("rejects a request with an invalid api key", async () => {
  const app = buildTestApp(stubService);
  const res = await app.inject({
    method: "GET",
    url: "/protected",
    headers: { authorization: "Bearer wrong-key" },
  });
  assert.equal(res.statusCode, 401);
});

test("resolves the account for a valid api key", async () => {
  const app = buildTestApp(stubService);
  const res = await app.inject({
    method: "GET",
    url: "/protected",
    headers: { authorization: "Bearer good-key" },
  });
  assert.equal(res.statusCode, 200);
  assert.deepEqual(res.json(), { accountId: fakeAccount.uuid });
});
