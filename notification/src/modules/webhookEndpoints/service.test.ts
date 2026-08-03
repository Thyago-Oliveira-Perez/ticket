import { test } from "node:test";
import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import type { WebhookEndpoint } from "@prisma/client";
import type { WebhookEndpointRepository } from "./repository.js";
import { WebhookEndpointServiceImpl } from "./service.js";
import { ValidationError } from "../../lib/errors.js";

class FakeWebhookEndpointRepository implements WebhookEndpointRepository {
  endpoints: WebhookEndpoint[] = [];

  async createEndpoint(accountId: string, url: string, secret: string): Promise<WebhookEndpoint> {
    const endpoint: WebhookEndpoint = {
      uuid: randomUUID(),
      accountId,
      url,
      secret,
      active: true,
      createdAt: new Date(),
      updatedAt: new Date(),
    };
    this.endpoints.push(endpoint);
    return endpoint;
  }

  async listActiveByAccount(accountId: string): Promise<WebhookEndpoint[]> {
    return this.endpoints.filter((e) => e.accountId === accountId && e.active);
  }
}

test("createEndpoint issues a signing secret", async () => {
  const service = new WebhookEndpointServiceImpl(new FakeWebhookEndpointRepository());
  const endpoint = await service.createEndpoint("acc-1", "https://example.com/hook");

  assert.match(endpoint.secret, /^whsec_[0-9a-f]{48}$/);
  assert.equal(endpoint.url, "https://example.com/hook");
});

test("createEndpoint rejects a non-http(s) url", async () => {
  const service = new WebhookEndpointServiceImpl(new FakeWebhookEndpointRepository());
  await assert.rejects(() => service.createEndpoint("acc-1", "ftp://example.com"), ValidationError);
});

test("createEndpoint rejects a malformed url", async () => {
  const service = new WebhookEndpointServiceImpl(new FakeWebhookEndpointRepository());
  await assert.rejects(() => service.createEndpoint("acc-1", "not a url"), ValidationError);
});
