import { createHash } from "node:crypto";
import type { FastifyInstance, FastifyReply, FastifyRequest } from "fastify";
import type { IdempotencyKey, PrismaClient } from "@prisma/client";
import { ConflictError, UnprocessableError } from "../lib/errors.js";

const HEADER = "idempotency-key";

export interface IdempotencyRepository {
  /**
   * Atomically inserts a placeholder record for (accountId, key), or — if
   * one already exists — returns it instead. `reserved` is true only if
   * this call created the record, meaning the caller now owns completing
   * it via `complete`.
   */
  reserve(
    accountId: string,
    key: string,
    requestHash: string,
  ): Promise<{ record: IdempotencyKey; reserved: boolean }>;
  /** Fills in the final response for a record previously reserved by this caller. */
  complete(id: string, responseStatus: number, responseBody: Buffer): Promise<void>;
}

export class PrismaIdempotencyRepository implements IdempotencyRepository {
  constructor(private readonly prisma: PrismaClient) {}

  async reserve(accountId: string, key: string, requestHash: string) {
    try {
      const record = await this.prisma.idempotencyKey.create({
        data: { accountId, key, requestHash },
      });
      return { record, reserved: true };
    } catch (err) {
      if (!isUniqueConstraintError(err)) throw err;

      // ON CONFLICT DO NOTHING equivalent: someone else already reserved this key.
      const existing = await this.prisma.idempotencyKey.findUniqueOrThrow({
        where: { accountId_key: { accountId, key } },
      });
      return { record: existing, reserved: false };
    }
  }

  async complete(id: string, responseStatus: number, responseBody: Buffer): Promise<void> {
    await this.prisma.idempotencyKey.update({
      where: { uuid: id },
      data: { responseStatus, responseBody: new Uint8Array(responseBody) },
    });
  }
}

function isUniqueConstraintError(err: unknown): boolean {
  return typeof err === "object" && err !== null && (err as { code?: string }).code === "P2002";
}

export function hashRequestBody(body: unknown): string {
  return createHash("sha256").update(canonicalize(body)).digest("hex");
}

// A stable stringify so key order in the client's JSON doesn't produce a
// spurious hash mismatch on retry.
function canonicalize(value: unknown): string {
  if (value === undefined || value === null) return "null";
  if (typeof value !== "object") return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(canonicalize).join(",")}]`;

  const entries = Object.entries(value as Record<string, unknown>).sort(([a], [b]) => (a < b ? -1 : 1));
  return `{${entries.map(([k, v]) => `${JSON.stringify(k)}:${canonicalize(v)}`).join(",")}}`;
}

declare module "fastify" {
  interface FastifyRequest {
    idempotencyReservationId?: string;
  }
}

/** Mounts the onSend hook that persists a reserved request's response. Call once per app. */
export function registerIdempotency(app: FastifyInstance, repo: IdempotencyRepository): void {
  app.decorateRequest("idempotencyReservationId", undefined);

  app.addHook("onSend", async (request, reply, payload) => {
    if (!request.idempotencyReservationId) return payload;

    const body = Buffer.isBuffer(payload) ? payload : Buffer.from(typeof payload === "string" ? payload : String(payload));
    await repo.complete(request.idempotencyReservationId, reply.statusCode, body);
    return payload;
  });
}

/**
 * Must be mounted behind auth (needs request.account) on any route that
 * accepts an Idempotency-Key. A request without the header passes through
 * untouched.
 */
export function idempotencyPreHandler(repo: IdempotencyRepository) {
  return async (request: FastifyRequest, reply: FastifyReply): Promise<void | FastifyReply> => {
    const key = request.headers[HEADER];
    if (!key || Array.isArray(key)) return;

    const accountId = request.account?.uuid;
    if (!accountId) return;

    const requestHash = hashRequestBody(request.body);
    const { record, reserved } = await repo.reserve(accountId, key, requestHash);

    if (reserved) {
      request.idempotencyReservationId = record.uuid;
      return;
    }

    if (record.requestHash !== requestHash) {
      throw new UnprocessableError("Idempotency-Key was already used with a different request body");
    }
    if (record.responseStatus === null) {
      throw new ConflictError("a request with this Idempotency-Key is already being processed");
    }

    return reply.type("application/json").code(record.responseStatus).send(record.responseBody ?? Buffer.alloc(0));
  };
}
