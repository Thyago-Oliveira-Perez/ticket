import { randomBytes } from "node:crypto";
import type { WebhookEndpoint } from "@prisma/client";
import type { WebhookEndpointRepository } from "./repository.js";
import { ValidationError } from "../../lib/errors.js";

export interface WebhookEndpointService {
  /** The returned endpoint's secret is populated only here, at creation time — it's never retrievable again. */
  createEndpoint(accountId: string, url: string): Promise<WebhookEndpoint>;
  listActiveByAccount(accountId: string): Promise<WebhookEndpoint[]>;
}

export class WebhookEndpointServiceImpl implements WebhookEndpointService {
  constructor(private readonly repo: WebhookEndpointRepository) {}

  async createEndpoint(accountId: string, url: string): Promise<WebhookEndpoint> {
    validateUrl(url);
    const secret = generateSecret();
    return this.repo.createEndpoint(accountId, url, secret);
  }

  async listActiveByAccount(accountId: string): Promise<WebhookEndpoint[]> {
    return this.repo.listActiveByAccount(accountId);
  }
}

function validateUrl(url: string): void {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    throw new ValidationError("url must be a valid http(s) URL");
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new ValidationError("url must be a valid http(s) URL");
  }
}

function generateSecret(): string {
  return `whsec_${randomBytes(24).toString("hex")}`;
}
