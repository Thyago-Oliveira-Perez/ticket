import Fastify, { type FastifyError, type FastifyInstance } from "fastify";
import type { PrismaClient } from "@prisma/client";
import type { Config } from "./config.js";
import { statusForError } from "./lib/errors.js";
import { PrismaAccountRepository } from "./modules/accounts/repository.js";
import { AccountServiceImpl } from "./modules/accounts/service.js";
import { registerAccountRoutes } from "./modules/accounts/routes.js";
import { PrismaWebhookEndpointRepository } from "./modules/webhookEndpoints/repository.js";
import { WebhookEndpointServiceImpl } from "./modules/webhookEndpoints/service.js";
import { registerWebhookEndpointRoutes } from "./modules/webhookEndpoints/routes.js";
import { PrismaSuppressionRepository } from "./modules/suppressions/repository.js";
import { SuppressionServiceImpl } from "./modules/suppressions/service.js";
import { registerSuppressionRoutes } from "./modules/suppressions/routes.js";
import { PrismaEventsRepository } from "./modules/events/repository.js";
import { EventPublisherImpl } from "./modules/events/publisher.js";
import { HttpWebhookSender } from "./modules/webhookDelivery/sender.js";
import { PrismaMessageRepository } from "./modules/messages/repository.js";
import { MessageServiceImpl } from "./modules/messages/service.js";
import { registerMessageRoutes } from "./modules/messages/routes.js";
import { registerAuth } from "./plugins/auth.js";
import { registerChaos } from "./plugins/chaos.js";
import { PrismaIdempotencyRepository, registerIdempotency } from "./plugins/idempotency.js";

function hasClientStatusCode(err: Error): err is FastifyError {
  const status = (err as Partial<FastifyError>).statusCode;
  return typeof status === "number" && status >= 400 && status < 500;
}

export function buildApp(config: Config, prisma: PrismaClient): FastifyInstance {
  const app = Fastify({ logger: true });

  app.setErrorHandler((err: Error, request, reply) => {
    const status = statusForError(err) ?? (hasClientStatusCode(err) ? err.statusCode : undefined);
    if (status) {
      reply.code(status).send({ error: err.message });
      return;
    }
    request.log.error(err);
    reply.code(500).send({ error: "internal server error" });
  });

  registerChaos(app, config.chaos);

  app.get("/health", async () => ({ status: "ok" }));

  const accountRepo = new PrismaAccountRepository(prisma);
  const accountService = new AccountServiceImpl(accountRepo);
  const idempotencyRepo = new PrismaIdempotencyRepository(prisma);

  const webhookEndpointRepo = new PrismaWebhookEndpointRepository(prisma);
  const webhookEndpointService = new WebhookEndpointServiceImpl(webhookEndpointRepo);

  const suppressionRepo = new PrismaSuppressionRepository(prisma);
  const suppressionService = new SuppressionServiceImpl(suppressionRepo);

  const eventsRepo = new PrismaEventsRepository(prisma);
  const webhookSender = new HttpWebhookSender(config.webhookChaos);
  const eventPublisher = new EventPublisherImpl(eventsRepo, webhookEndpointRepo, webhookSender);

  const messageRepo = new PrismaMessageRepository(prisma);
  const messageService = new MessageServiceImpl(prisma, messageRepo, suppressionRepo, eventPublisher, config.messageChaos);

  registerAuth(app, accountService);
  registerIdempotency(app, idempotencyRepo);
  registerAccountRoutes(app, accountService);
  registerWebhookEndpointRoutes(app, webhookEndpointService);
  registerSuppressionRoutes(app, suppressionService);
  registerMessageRoutes(app, messageService, idempotencyRepo);

  return app;
}
