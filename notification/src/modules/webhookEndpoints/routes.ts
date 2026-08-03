import type { FastifyInstance } from "fastify";
import type { WebhookEndpoint } from "@prisma/client";
import type { WebhookEndpointService } from "./service.js";

function serializeEndpoint(endpoint: WebhookEndpoint) {
  return {
    id: endpoint.uuid,
    url: endpoint.url,
    active: endpoint.active,
    created_at: endpoint.createdAt,
    updated_at: endpoint.updatedAt,
  };
}

export function registerWebhookEndpointRoutes(
  app: FastifyInstance,
  service: WebhookEndpointService,
): void {
  app.post<{ Body: { url?: string } }>(
    "/webhook-endpoints",
    { preHandler: app.authenticate },
    async (request, reply) => {
      const endpoint = await service.createEndpoint(request.account!.uuid, request.body?.url ?? "");
      return reply.code(201).send({ ...serializeEndpoint(endpoint), secret: endpoint.secret });
    },
  );
}
