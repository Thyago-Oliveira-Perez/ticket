import Fastify, { type FastifyError, type FastifyInstance } from "fastify";
import type { PrismaClient } from "@prisma/client";
import type { Config } from "./config.js";
import { statusForError } from "./lib/errors.js";

function hasClientStatusCode(err: Error): err is FastifyError {
  const status = (err as Partial<FastifyError>).statusCode;
  return typeof status === "number" && status >= 400 && status < 500;
}
import { PrismaAccountRepository } from "./modules/accounts/repository.js";
import { AccountServiceImpl } from "./modules/accounts/service.js";
import { registerAccountRoutes } from "./modules/accounts/routes.js";
import { registerAuth } from "./plugins/auth.js";

export function buildApp(config: Config, prisma: PrismaClient): FastifyInstance {
  const app = Fastify({ logger: true });

  void config;

  app.setErrorHandler((err: Error, request, reply) => {
    const status = statusForError(err) ?? (hasClientStatusCode(err) ? err.statusCode : undefined);
    if (status) {
      reply.code(status).send({ error: err.message });
      return;
    }
    request.log.error(err);
    reply.code(500).send({ error: "internal server error" });
  });

  app.get("/health", async () => ({ status: "ok" }));

  const accountRepo = new PrismaAccountRepository(prisma);
  const accountService = new AccountServiceImpl(accountRepo);

  registerAuth(app, accountService);
  registerAccountRoutes(app, accountService);

  return app;
}
