import type { FastifyInstance } from "fastify";
import type { Suppression } from "@prisma/client";
import type { SuppressionService } from "./service.js";

function serializeSuppression(suppression: Suppression) {
  return {
    id: suppression.uuid,
    channel: suppression.channel,
    address: suppression.address,
    reason: suppression.reason,
    created_at: suppression.createdAt,
  };
}

export function registerSuppressionRoutes(app: FastifyInstance, service: SuppressionService): void {
  app.get("/suppressions", { preHandler: app.authenticate }, async (request) => {
    const suppressions = await service.listByAccount(request.account!.uuid);
    return suppressions.map(serializeSuppression);
  });
}
