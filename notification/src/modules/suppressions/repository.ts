import type { Channel, PrismaClient, Suppression, SuppressionReason } from "@prisma/client";
import type { Db } from "../../lib/prisma.js";

export interface SuppressionRepository {
  isSuppressed(accountId: string, channel: Channel, address: string): Promise<boolean>;
  /**
   * Idempotent: suppressing an already-suppressed address is a no-op,
   * keeping the earliest reason. Runs via db so it can participate in the
   * same transaction as the state change that triggered it (e.g. a bounce).
   */
  upsert(db: Db, accountId: string, channel: Channel, address: string, reason: SuppressionReason): Promise<Suppression>;
  listByAccount(accountId: string): Promise<Suppression[]>;
}

export class PrismaSuppressionRepository implements SuppressionRepository {
  constructor(private readonly prisma: PrismaClient) {}

  async isSuppressed(accountId: string, channel: Channel, address: string): Promise<boolean> {
    const existing = await this.prisma.suppression.findUnique({
      where: { accountId_channel_address: { accountId, channel, address } },
    });
    return existing !== null;
  }

  async upsert(db: Db, accountId: string, channel: Channel, address: string, reason: SuppressionReason): Promise<Suppression> {
    return db.suppression.upsert({
      where: { accountId_channel_address: { accountId, channel, address } },
      create: { accountId, channel, address, reason },
      update: {},
    });
  }

  async listByAccount(accountId: string): Promise<Suppression[]> {
    return this.prisma.suppression.findMany({ where: { accountId }, orderBy: { createdAt: "desc" } });
  }
}
