import type { PrismaClient, WebhookEndpoint } from "@prisma/client";

export interface WebhookEndpointRepository {
  createEndpoint(accountId: string, url: string, secret: string): Promise<WebhookEndpoint>;
  listActiveByAccount(accountId: string): Promise<WebhookEndpoint[]>;
}

export class PrismaWebhookEndpointRepository implements WebhookEndpointRepository {
  constructor(private readonly prisma: PrismaClient) {}

  async createEndpoint(accountId: string, url: string, secret: string): Promise<WebhookEndpoint> {
    return this.prisma.webhookEndpoint.create({ data: { accountId, url, secret } });
  }

  async listActiveByAccount(accountId: string): Promise<WebhookEndpoint[]> {
    return this.prisma.webhookEndpoint.findMany({ where: { accountId, active: true } });
  }
}
