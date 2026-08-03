import { test } from "node:test";
import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import type { Channel, Message, MessageStatus, Suppression, SuppressionReason } from "@prisma/client";
import type { Db } from "../../lib/prisma.js";
import type { CreateMessageInput, MessageRepository } from "./repository.js";
import type { SuppressionRepository } from "../suppressions/repository.js";
import type { Delivery, EventPublisher } from "../events/publisher.js";
import { MessageServiceImpl } from "./service.js";
import { NotFoundError, ValidationError } from "../../lib/errors.js";

const testAccountId = "11111111-1111-1111-1111-111111111111";

class FakeMessageRepository implements MessageRepository {
  messages: Message[] = [];

  async create(_db: Db, input: CreateMessageInput): Promise<Message> {
    const message: Message = {
      uuid: randomUUID(),
      providerMessageId: randomUUID(),
      createdAt: new Date(),
      updatedAt: new Date(),
      ...input,
    };
    this.messages.push(message);
    return message;
  }

  async updateStatus(_db: Db, id: string, status: MessageStatus): Promise<Message> {
    const message = this.messages.find((m) => m.uuid === id);
    if (!message) throw new Error("not found");
    message.status = status;
    return message;
  }

  async listByAccount(accountId: string): Promise<Message[]> {
    return this.messages.filter((m) => m.accountId === accountId);
  }

  async getById(accountId: string, id: string): Promise<Message | null> {
    return this.messages.find((m) => m.accountId === accountId && m.uuid === id) ?? null;
  }
}

class FakeSuppressionRepository implements SuppressionRepository {
  suppressions: Suppression[] = [];

  async isSuppressed(accountId: string, channel: Channel, address: string): Promise<boolean> {
    return this.suppressions.some((s) => s.accountId === accountId && s.channel === channel && s.address === address);
  }

  async upsert(_db: Db, accountId: string, channel: Channel, address: string, reason: SuppressionReason): Promise<Suppression> {
    const suppression: Suppression = { uuid: randomUUID(), accountId, channel, address, reason, createdAt: new Date() };
    this.suppressions.push(suppression);
    return suppression;
  }

  async listByAccount(accountId: string): Promise<Suppression[]> {
    return this.suppressions.filter((s) => s.accountId === accountId);
  }
}

class FakeEventPublisher implements EventPublisher {
  eventTypes: string[] = [];

  async publish(_db: Db, _accountId: string, type: string): Promise<Delivery[]> {
    this.eventTypes.push(type);
    return [];
  }

  dispatch(): void {}
}

const fakeDb = {} as Db;
const fakePrisma = { $transaction: (fn: (tx: Db) => Promise<unknown>) => fn(fakeDb) } as unknown as import("@prisma/client").PrismaClient;

const noChaos = { bounceRate: 0, sendDelayMinMs: 1, sendDelayMaxMs: 2, deliverDelayMinMs: 1, deliverDelayMaxMs: 2 };

function waitForLifecycle(ms = 40): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

test("sendMessage validates required fields", async () => {
  const service = new MessageServiceImpl(
    fakePrisma,
    new FakeMessageRepository(),
    new FakeSuppressionRepository(),
    new FakeEventPublisher(),
    noChaos,
  );

  await assert.rejects(() => service.sendMessage(testAccountId, { channel: "carrier-pigeon" as never, to: "a", from: "b", body: "c" }), ValidationError);
  await assert.rejects(() => service.sendMessage(testAccountId, { channel: "email", from: "b", body: "c" }), ValidationError);
  await assert.rejects(() => service.sendMessage(testAccountId, { channel: "email", to: "a", body: "c" }), ValidationError);
  await assert.rejects(() => service.sendMessage(testAccountId, { channel: "email", to: "a", from: "b" }), ValidationError);
});

test("sendMessage queues a message and it lands delivered when never bounced", async () => {
  const repo = new FakeMessageRepository();
  const publisher = new FakeEventPublisher();
  const service = new MessageServiceImpl(
    fakePrisma,
    repo,
    new FakeSuppressionRepository(),
    publisher,
    noChaos,
    () => false,
  );

  const message = await service.sendMessage(testAccountId, {
    channel: "email",
    to: "buyer@example.com",
    from: "orders@example.com",
    subject: "Your ticket",
    body: "Enjoy the show",
  });
  assert.equal(message.status, "queued");

  await waitForLifecycle();

  const stored = await repo.getById(testAccountId, message.uuid);
  assert.equal(stored?.status, "delivered");
  assert.deepEqual(publisher.eventTypes, ["message.queued", "message.sent", "message.delivered"]);
});

test("an email that bounces transitions to bounced and suppresses the address", async () => {
  const repo = new FakeMessageRepository();
  const suppressions = new FakeSuppressionRepository();
  const publisher = new FakeEventPublisher();
  const service = new MessageServiceImpl(fakePrisma, repo, suppressions, publisher, noChaos, () => true);

  const message = await service.sendMessage(testAccountId, {
    channel: "email",
    to: "bounce@example.com",
    from: "orders@example.com",
    body: "Enjoy the show",
  });

  await waitForLifecycle();

  const stored = await repo.getById(testAccountId, message.uuid);
  assert.equal(stored?.status, "bounced");
  assert.deepEqual(publisher.eventTypes, ["message.queued", "message.sent", "message.bounced"]);
  assert.equal(await suppressions.isSuppressed(testAccountId, "email", "bounce@example.com"), true);
});

test("an sms that fails transitions to failed without suppressing the number", async () => {
  const repo = new FakeMessageRepository();
  const suppressions = new FakeSuppressionRepository();
  const publisher = new FakeEventPublisher();
  const service = new MessageServiceImpl(fakePrisma, repo, suppressions, publisher, noChaos, () => true);

  const message = await service.sendMessage(testAccountId, {
    channel: "sms",
    to: "+15551234",
    from: "+15555678",
    body: "Enjoy the show",
  });

  await waitForLifecycle();

  const stored = await repo.getById(testAccountId, message.uuid);
  assert.equal(stored?.status, "failed");
  assert.deepEqual(publisher.eventTypes, ["message.queued", "message.sent", "message.failed"]);
  assert.equal(await suppressions.isSuppressed(testAccountId, "sms", "+15551234"), false);
});

test("sending to a suppressed address short-circuits to suppressed with no further lifecycle", async () => {
  const repo = new FakeMessageRepository();
  const suppressions = new FakeSuppressionRepository();
  await suppressions.upsert(fakeDb, testAccountId, "email", "blocked@example.com", "bounced");
  const publisher = new FakeEventPublisher();
  const service = new MessageServiceImpl(fakePrisma, repo, suppressions, publisher, noChaos, () => {
    throw new Error("should not roll for a suppressed send");
  });

  const message = await service.sendMessage(testAccountId, {
    channel: "email",
    to: "blocked@example.com",
    from: "orders@example.com",
    body: "Enjoy the show",
  });
  assert.equal(message.status, "suppressed");

  await waitForLifecycle();

  const stored = await repo.getById(testAccountId, message.uuid);
  assert.equal(stored?.status, "suppressed");
  assert.deepEqual(publisher.eventTypes, ["message.suppressed"]);
});

test("getById returns 404 for an unknown message", async () => {
  const service = new MessageServiceImpl(
    fakePrisma,
    new FakeMessageRepository(),
    new FakeSuppressionRepository(),
    new FakeEventPublisher(),
    noChaos,
  );
  await assert.rejects(() => service.getById(testAccountId, randomUUID()), NotFoundError);
});

test("getById returns 400 for a malformed id", async () => {
  const service = new MessageServiceImpl(
    fakePrisma,
    new FakeMessageRepository(),
    new FakeSuppressionRepository(),
    new FakeEventPublisher(),
    noChaos,
  );
  await assert.rejects(() => service.getById(testAccountId, "not-a-uuid"), ValidationError);
});
