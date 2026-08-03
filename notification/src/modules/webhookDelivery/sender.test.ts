import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type Server } from "node:http";
import { HttpWebhookSender } from "./sender.js";
import type { WebhookChaosConfig } from "../../config.js";

const noChaos: WebhookChaosConfig = { latencyMinMs: 0, latencyMaxMs: 0, duplicateRate: 0 };

interface Received {
  eventId: string | undefined;
  signature: string | undefined;
  body: { id: string; sequence: number; type: string; data: unknown };
}

async function withCapturingServer(fn: (url: string, received: Received[]) => Promise<void>): Promise<void> {
  const received: Received[] = [];
  const server: Server = createServer((req, res) => {
    let raw = "";
    req.on("data", (chunk) => (raw += chunk));
    req.on("end", () => {
      received.push({
        eventId: req.headers["x-webhook-event-id"] as string | undefined,
        signature: req.headers["x-webhook-signature"] as string | undefined,
        body: JSON.parse(raw),
      });
      res.writeHead(200);
      res.end();
    });
  });

  await new Promise<void>((resolve) => server.listen(0, resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("expected a network address");

  try {
    await fn(`http://127.0.0.1:${address.port}`, received);
  } finally {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
}

test("send delivers once by default", async () => {
  await withCapturingServer(async (url, received) => {
    const sender = new HttpWebhookSender(noChaos);
    await sender.send(url, "whsec_test", "message.sent", { id: "m1" });

    assert.equal(received.length, 1);
    assert.equal(received[0].body.type, "message.sent");
    assert.equal(received[0].eventId, received[0].body.id);
  });
});

test("send signs the request with a Stripe-style header", async () => {
  await withCapturingServer(async (url, received) => {
    const sender = new HttpWebhookSender(noChaos);
    await sender.send(url, "whsec_test", "message.sent", { id: "m1" });

    assert.match(received[0].signature ?? "", /^t=\d+,v1=[0-9a-f]+$/);
  });
});

test("duplicate rate 1 sends twice with the same event id", async () => {
  await withCapturingServer(async (url, received) => {
    const sender = new HttpWebhookSender({ ...noChaos, duplicateRate: 1 });
    await sender.send(url, "whsec_test", "message.sent", { id: "m1" });

    assert.equal(received.length, 2);
    assert.equal(received[0].body.id, received[1].body.id);
  });
});

test("sequence increases across calls", async () => {
  await withCapturingServer(async (url, received) => {
    const sender = new HttpWebhookSender(noChaos);
    for (let i = 0; i < 3; i++) {
      await sender.send(url, "whsec_test", "message.sent", null);
    }

    assert.equal(received.length, 3);
    for (let i = 1; i < received.length; i++) {
      assert.ok(received[i].body.sequence > received[i - 1].body.sequence);
    }
  });
});
