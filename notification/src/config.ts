function envInt(name: string, fallback: number): number {
  const raw = process.env[name];
  if (raw === undefined || raw === "") return fallback;
  const value = Number.parseInt(raw, 10);
  if (Number.isNaN(value)) throw new Error(`${name} must be an integer, got "${raw}"`);
  return value;
}

function envFloat(name: string, fallback: number): number {
  const raw = process.env[name];
  if (raw === undefined || raw === "") return fallback;
  const value = Number.parseFloat(raw);
  if (Number.isNaN(value)) throw new Error(`${name} must be a number, got "${raw}"`);
  return value;
}

export interface ChaosConfig {
  latencyMinMs: number;
  latencyMaxMs: number;
  errorRate: number;
  rateLimitRps: number;
  rateLimitBurst: number;
}

export interface WebhookChaosConfig {
  latencyMinMs: number;
  latencyMaxMs: number;
  duplicateRate: number;
}

export interface MessageChaosConfig {
  bounceRate: number;
  sendDelayMinMs: number;
  sendDelayMaxMs: number;
  deliverDelayMinMs: number;
  deliverDelayMaxMs: number;
}

export interface Config {
  addr: string;
  port: number;
  dbDsn: string;
  chaos: ChaosConfig;
  webhookChaos: WebhookChaosConfig;
  messageChaos: MessageChaosConfig;
}

export function loadConfig(env: NodeJS.ProcessEnv = process.env): Config {
  const port = envInt("PORT", 3000);
  const rateLimitRps = envInt("CHAOS_RATE_LIMIT_RPS", 0);

  return {
    addr: env.ADDR ?? "0.0.0.0",
    port,
    dbDsn: env.DB_DSN ?? "",
    chaos: {
      latencyMinMs: envInt("CHAOS_LATENCY_MIN_MS", 0),
      latencyMaxMs: envInt("CHAOS_LATENCY_MAX_MS", 0),
      errorRate: envFloat("CHAOS_ERROR_RATE", 0),
      rateLimitRps,
      rateLimitBurst: envInt("CHAOS_RATE_LIMIT_BURST", Math.ceil(rateLimitRps)),
    },
    webhookChaos: {
      latencyMinMs: envInt("CHAOS_WEBHOOK_LATENCY_MIN_MS", 0),
      latencyMaxMs: envInt("CHAOS_WEBHOOK_LATENCY_MAX_MS", 0),
      duplicateRate: envFloat("CHAOS_WEBHOOK_DUPLICATE_RATE", 0),
    },
    messageChaos: {
      bounceRate: envFloat("MESSAGE_BOUNCE_RATE", 0),
      sendDelayMinMs: envInt("MESSAGE_SEND_DELAY_MIN_MS", 0),
      sendDelayMaxMs: envInt("MESSAGE_SEND_DELAY_MAX_MS", 0),
      deliverDelayMinMs: envInt("MESSAGE_DELIVER_DELAY_MIN_MS", 0),
      deliverDelayMaxMs: envInt("MESSAGE_DELIVER_DELAY_MAX_MS", 0),
    },
  };
}
