import { test } from "node:test";
import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import type { Event, WebhookDelivery, WebhookEndpoint } from "@prisma/client";
import type { Db } from "../../lib/prisma.js";
import type { EventsRepository } from "./repository.js";
import type { WebhookEndpointRepository } from "../webhookEndpoints/repository.js";
import type { WebhookSender } from "../webhookDelivery/sender.js";
import { EventPublisherImpl } from "./publisher.js";

class FakeEventsRepository implements EventsRepository {
  events: Event[] = [];
  deliveries: WebhookDelivery[] = [];
  statusUpdates: { id: string; status: string; attempts: number }[] = [];

  async insertEvent(_db: Db, accountId: string, type: string, resourceId: string, payload: unknown): Promise<Event> {
    const event: Event = { uuid: randomUUID(), accountId, type, resourceId, payload: payload as never, createdAt: new Date() };
    this.events.push(event);
    return event;
  }

  async insertDelivery(_db: Db, endpointId: string, eventId: string): Promise<WebhookDelivery> {
    const delivery: WebhookDelivery = {
      uuid: randomUUID(),
      endpointId,
      eventId,
      attempts: 0,
      status: "pending",
      createdAt: new Date(),
      updatedAt: new Date(),
    };
    this.deliveries.push(delivery);
    return delivery;
  }

  async updateDeliveryStatus(id: string, status: "delivered" | "failed", attempts: number): Promise<void> {
    this.statusUpdates.push({ id, status, attempts });
  }
}

class FakeWebhookEndpointRepository implements WebhookEndpointRepository {
  constructor(private readonly endpoints: WebhookEndpoint[]) {}
  async createEndpoint(): Promise<WebhookEndpoint> {
    throw new Error("not used in this test");
  }
  async listActiveByAccount(accountId: string): Promise<WebhookEndpoint[]> {
    return this.endpoints.filter((e) => e.accountId === accountId && e.active);
  }
}

function makeEndpoint(accountId: string, active = true): WebhookEndpoint {
  return {
    uuid: randomUUID(),
    accountId,
    url: "https://example.com/hook",
    secret: "whsec_test",
    active,
    createdAt: new Date(),
    updatedAt: new Date(),
  };
}

class FakeSender implements WebhookSender {
  calls: { url: string; secret: string; eventType: string; data: unknown }[] = [];
  shouldFail = false;

  async send(url: string, secret: string, eventType: string, data: unknown): Promise<void> {
    this.calls.push({ url, secret, eventType, data });
    if (this.shouldFail) throw new Error("delivery failed");
  }
}

const fakeDb = {} as Db;

test("publish writes one delivery per active endpoint", async () => {
  const endpointA = makeEndpoint("acc-1");
  const endpointB = makeEndpoint("acc-1");
  const inactiveEndpoint = makeEndpoint("acc-1", false);
  const otherAccountEndpoint = makeEndpoint("acc-2");

  const eventsRepo = new FakeEventsRepository();
  const endpointsRepo = new FakeWebhookEndpointRepository([endpointA, endpointB, inactiveEndpoint, otherAccountEndpoint]);
  const publisher = new EventPublisherImpl(eventsRepo, endpointsRepo, new FakeSender());

  const deliveries = await publisher.publish(fakeDb, "acc-1", "message.sent", "msg-1", { id: "msg-1" });

  assert.equal(deliveries.length, 2);
  assert.equal(eventsRepo.events.length, 1);
  assert.equal(eventsRepo.events[0]?.type, "message.sent");
});

test("dispatch delivers and marks each delivery delivered", async () => {
  const endpoint = makeEndpoint("acc-1");
  const eventsRepo = new FakeEventsRepository();
  const endpointsRepo = new FakeWebhookEndpointRepository([endpoint]);
  const sender = new FakeSender();
  const publisher = new EventPublisherImpl(eventsRepo, endpointsRepo, sender);

  const deliveries = await publisher.publish(fakeDb, "acc-1", "message.sent", "msg-1", { id: "msg-1" });
  publisher.dispatch(deliveries);

  // dispatch is fire-and-forget; give its microtasks a turn to run.
  await new Promise((resolve) => setTimeout(resolve, 10));

  assert.equal(sender.calls.length, 1);
  assert.equal(sender.calls[0]?.eventType, "message.sent");
  assert.deepEqual(eventsRepo.statusUpdates, [{ id: deliveries[0]!.id, status: "delivered", attempts: 1 }]);
});

test("dispatch marks a delivery failed when the sender throws", async () => {
  const endpoint = makeEndpoint("acc-1");
  const eventsRepo = new FakeEventsRepository();
  const endpointsRepo = new FakeWebhookEndpointRepository([endpoint]);
  const sender = new FakeSender();
  sender.shouldFail = true;
  const publisher = new EventPublisherImpl(eventsRepo, endpointsRepo, sender);

  const deliveries = await publisher.publish(fakeDb, "acc-1", "message.sent", "msg-1", { id: "msg-1" });
  publisher.dispatch(deliveries);

  await new Promise((resolve) => setTimeout(resolve, 10));

  assert.deepEqual(eventsRepo.statusUpdates, [{ id: deliveries[0]!.id, status: "failed", attempts: 1 }]);
});
