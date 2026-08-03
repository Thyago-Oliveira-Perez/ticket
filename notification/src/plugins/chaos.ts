import type { FastifyInstance, FastifyReply, FastifyRequest } from "fastify";
import type { ChaosConfig } from "../config.js";

type Hook = (request: FastifyRequest, reply: FastifyReply) => Promise<void>;

function randomDurationMs(min: number, max: number): number {
  if (max <= min) return min;
  return min + Math.floor(Math.random() * (max - min));
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Delays each request by a random duration in [minMs, maxMs). A no-op when maxMs <= 0. */
export function latencyHook(cfg: Pick<ChaosConfig, "latencyMinMs" | "latencyMaxMs">): Hook {
  return async (_request, _reply) => {
    if (cfg.latencyMaxMs <= 0) return;
    await sleep(randomDurationMs(cfg.latencyMinMs, cfg.latencyMaxMs));
  };
}

/** Fails a fraction of requests, set by cfg.errorRate, with a 500. A no-op when errorRate <= 0. */
export function randomErrorHook(cfg: Pick<ChaosConfig, "errorRate">): Hook {
  return async (_request, reply) => {
    if (cfg.errorRate <= 0) return;
    if (Math.random() < cfg.errorRate) {
      reply.code(500).send({ error: "internal server error" });
    }
  };
}

class TokenBucket {
  private tokens: number;
  private lastRefillMs: number;

  constructor(private readonly ratePerSec: number, private readonly capacity: number) {
    this.tokens = capacity;
    this.lastRefillMs = Date.now();
  }

  take(): boolean {
    const now = Date.now();
    const elapsedSec = (now - this.lastRefillMs) / 1000;
    this.tokens = Math.min(this.capacity, this.tokens + elapsedSec * this.ratePerSec);
    this.lastRefillMs = now;

    if (this.tokens < 1) return false;
    this.tokens -= 1;
    return true;
  }
}

/** A token bucket per key (client IP), so each client is throttled independently. */
export class PerKeyRateLimiter {
  private readonly buckets = new Map<string, TokenBucket>();

  constructor(private readonly ratePerSec: number, private readonly burst: number) {}

  allow(key: string): boolean {
    let bucket = this.buckets.get(key);
    if (!bucket) {
      bucket = new TokenBucket(this.ratePerSec, this.burst);
      this.buckets.set(key, bucket);
    }
    return bucket.take();
  }
}

/** Throttles requests per client IP with a token bucket, returning 429 once exhausted. A no-op when rateLimitRps <= 0. */
export function rateLimitHook(cfg: Pick<ChaosConfig, "rateLimitRps" | "rateLimitBurst">): Hook {
  if (cfg.rateLimitRps <= 0) {
    return async () => {};
  }

  const burst = cfg.rateLimitBurst > 0 ? cfg.rateLimitBurst : Math.max(1, Math.ceil(cfg.rateLimitRps));
  const limiter = new PerKeyRateLimiter(cfg.rateLimitRps, burst);

  return async (request, reply) => {
    if (!limiter.allow(request.ip)) {
      reply.code(429).send({ error: "rate limit exceeded" });
    }
  };
}

/**
 * Mounts chaos middleware globally. Order matters: rate limiting runs first
 * (so throttled clients don't pay the injected latency), then latency, then
 * the random failure check.
 */
export function registerChaos(app: FastifyInstance, cfg: ChaosConfig): void {
  app.addHook("onRequest", rateLimitHook(cfg));
  app.addHook("onRequest", latencyHook(cfg));
  app.addHook("onRequest", randomErrorHook(cfg));
}
