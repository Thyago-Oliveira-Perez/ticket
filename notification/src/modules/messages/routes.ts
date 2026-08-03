import type { FastifyInstance } from "fastify";
import type { MessageService, SendMessageInput } from "./service.js";
import { serializeMessage } from "./service.js";
import { idempotencyPreHandler, type IdempotencyRepository } from "../../plugins/idempotency.js";

export function registerMessageRoutes(
  app: FastifyInstance,
  service: MessageService,
  idempotencyRepo: IdempotencyRepository,
): void {
  app.post<{ Body: SendMessageInput }>(
    "/messages",
    { preHandler: [app.authenticate, idempotencyPreHandler(idempotencyRepo)] },
    async (request, reply) => {
      const message = await service.sendMessage(request.account!.uuid, request.body ?? {});
      return reply.code(201).send(serializeMessage(message));
    },
  );

  app.get("/messages", { preHandler: app.authenticate }, async (request) => {
    const messages = await service.listByAccount(request.account!.uuid);
    return messages.map(serializeMessage);
  });

  app.get<{ Params: { id: string } }>("/messages/:id", { preHandler: app.authenticate }, async (request) => {
    const message = await service.getById(request.account!.uuid, request.params.id);
    return serializeMessage(message);
  });
}
