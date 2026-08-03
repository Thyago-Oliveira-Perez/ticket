import type { FastifyInstance, FastifyReply, FastifyRequest } from "fastify";
import type { Account } from "@prisma/client";
import type { AccountService } from "../modules/accounts/service.js";
import { UnauthorizedError } from "../lib/errors.js";

declare module "fastify" {
  interface FastifyRequest {
    account: Account | null;
  }
  interface FastifyInstance {
    authenticate: (request: FastifyRequest, reply: FastifyReply) => Promise<void>;
  }
}

function bearerToken(header: string | undefined): string | undefined {
  if (!header?.startsWith("Bearer ")) return undefined;
  const token = header.slice("Bearer ".length).trim();
  return token === "" ? undefined : token;
}

export function registerAuth(app: FastifyInstance, service: AccountService): void {
  app.decorateRequest("account", null);

  app.decorate("authenticate", async (request: FastifyRequest) => {
    const rawKey = bearerToken(request.headers.authorization);
    if (!rawKey) throw new UnauthorizedError("missing or malformed Authorization header");

    request.account = await service.authenticateApiKey(rawKey);
  });
}
