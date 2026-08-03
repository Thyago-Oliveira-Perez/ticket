import { test } from "node:test";
import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import type { Channel, Suppression, SuppressionReason } from "@prisma/client";
import type { SuppressionRepository } from "./repository.js";
import { SuppressionServiceImpl } from "./service.js";

class FakeSuppressionRepository implements SuppressionRepository {
  suppressions: Suppression[] = [];

  async isSuppressed(accountId: string, channel: Channel, address: string): Promise<boolean> {
    return this.suppressions.some((s) => s.accountId === accountId && s.channel === channel && s.address === address);
  }

  async upsert(accountId: string, channel: Channel, address: string, reason: SuppressionReason): Promise<Suppression> {
    const existing = this.suppressions.find(
      (s) => s.accountId === accountId && s.channel === channel && s.address === address,
    );
    if (existing) return existing;

    const suppression: Suppression = { uuid: randomUUID(), accountId, channel, address, reason, createdAt: new Date() };
    this.suppressions.push(suppression);
    return suppression;
  }

  async listByAccount(accountId: string): Promise<Suppression[]> {
    return this.suppressions.filter((s) => s.accountId === accountId);
  }
}

test("upsert is idempotent for the same (account, channel, address)", async () => {
  const repo = new FakeSuppressionRepository();

  const first = await repo.upsert("acc-1", "email", "bounced@example.com", "bounced");
  const second = await repo.upsert("acc-1", "email", "bounced@example.com", "complained");

  assert.equal(first.uuid, second.uuid);
  assert.equal(second.reason, "bounced");
  assert.equal(await repo.isSuppressed("acc-1", "email", "bounced@example.com"), true);
});

test("listByAccount scopes to the account", async () => {
  const repo = new FakeSuppressionRepository();
  await repo.upsert("acc-1", "email", "a@example.com", "bounced");
  await repo.upsert("acc-2", "email", "b@example.com", "bounced");

  const service = new SuppressionServiceImpl(repo);
  const list = await service.listByAccount("acc-1");

  assert.equal(list.length, 1);
  assert.equal(list[0]?.address, "a@example.com");
});

test("isSuppressed is false for an address never suppressed", async () => {
  const repo = new FakeSuppressionRepository();
  assert.equal(await repo.isSuppressed("acc-1", "sms", "+15551234"), false);
});
