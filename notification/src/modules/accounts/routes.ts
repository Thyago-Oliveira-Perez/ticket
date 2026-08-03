import type { FastifyInstance } from "fastify";
import type { Account } from "@prisma/client";
import type { AccountService } from "./service.js";

function serializeAccount(account: Account) {
  return {
    id: account.uuid,
    name: account.name,
    status: account.status,
    created_at: account.createdAt,
    updated_at: account.updatedAt,
  };
}

export function registerAccountRoutes(app: FastifyInstance, service: AccountService): void {
  app.post<{ Body: { name?: string } }>("/accounts", async (request, reply) => {
    const { account, apiKey } = await service.createAccount(request.body?.name ?? "");
    return reply.code(201).send({ ...serializeAccount(account), api_key: apiKey });
  });
}
