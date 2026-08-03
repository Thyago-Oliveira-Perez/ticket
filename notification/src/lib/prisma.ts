import { Prisma, PrismaClient } from "@prisma/client";

export const prisma = new PrismaClient();

/** Satisfied by both the top-level client and a `$transaction` callback's client, so repositories can run inside a caller's transaction. */
export type Db = PrismaClient | Prisma.TransactionClient;
