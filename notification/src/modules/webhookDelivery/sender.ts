import { createHmac, randomUUID } from "node:crypto";
import type { WebhookChaosConfig } from "../../config.js";

interface Envelope {
  id: string;
  sequence: number;
  type: string;
  occurred_at: string;
  data: unknown;
}

export interface WebhookSender {
  /**
   * Delivers a `type`/`data` event to `url` as a signed JSON webhook
   * (Stripe-style: `t=<unix>,v1=<hmac>` over `<unix>.<body>`). Blocks for
   * the simulated latency and any duplicate delivery, so callers wanting
   * fire-and-forget semantics should not await it inline on a hot path.
   * Delivery failures (network errors, non-2xx responses) are logged, not
   * thrown — there's no retry beyond the configured duplicate rate.
   */
  send(url: string, secret: string, eventType: string, data: unknown): Promise<void>;
}

export class HttpWebhookSender implements WebhookSender {
  private sequence = 0;

  constructor(private readonly cfg: WebhookChaosConfig) {}

  async send(url: string, secret: string, eventType: string, data: unknown): Promise<void> {
    const id = randomUUID();
    const envelope: Envelope = {
      id,
      sequence: ++this.sequence,
      type: eventType,
      occurred_at: new Date().toISOString(),
      data,
    };
    const body = JSON.stringify(envelope);

    await this.deliver(url, secret, id, body);

    if (this.cfg.duplicateRate > 0 && Math.random() < this.cfg.duplicateRate) {
      await this.deliver(url, secret, id, body);
    }
  }

  private async deliver(url: string, secret: string, eventId: string, body: string): Promise<void> {
    await this.sleepLatency();

    try {
      const res = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Webhook-Event-Id": eventId,
          "X-Webhook-Signature": sign(secret, body),
        },
        body,
        signal: AbortSignal.timeout(10_000),
      });
      if (!res.ok) {
        console.error(`webhook: event ${eventId} to ${url} got status ${res.status}`);
      }
    } catch (err) {
      console.error(`webhook: deliver event ${eventId} to ${url}:`, err);
    }
  }

  private async sleepLatency(): Promise<void> {
    if (this.cfg.latencyMaxMs <= 0) return;
    const min = this.cfg.latencyMinMs;
    const max = this.cfg.latencyMaxMs;
    const delay = max > min ? min + Math.floor(Math.random() * (max - min)) : min;
    await new Promise((resolve) => setTimeout(resolve, delay));
  }
}

// Stripe-style signature: a timestamp plus an HMAC-SHA256 of
// "<timestamp>.<body>" keyed by secret, so a receiver can verify both
// authenticity and (via the timestamp) freshness.
function sign(secret: string, body: string): string {
  const ts = Math.floor(Date.now() / 1000);
  const mac = createHmac("sha256", secret).update(`${ts}.${body}`).digest("hex");
  return `t=${ts},v1=${mac}`;
}
