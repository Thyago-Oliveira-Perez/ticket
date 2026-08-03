import type { Event, PrismaClient, WebhookDelivery } from "@prisma/client";
import type { Db } from "../../lib/prisma.js";

export interface EventsRepository {
  /** Writes e via db, so it can participate in the same transaction as the state change it describes (the outbox insert). */
  insertEvent(db: Db, accountId: string, type: string, resourceId: string, payload: unknown): Promise<Event>;
  /** Writes one pending delivery row via db, alongside the event insert. */
  insertDelivery(db: Db, endpointId: string, eventId: string): Promise<WebhookDelivery>;
  /** Records the outcome of a delivery attempt, made after the transaction that created the row has already committed. */
  updateDeliveryStatus(id: string, status: "delivered" | "failed", attempts: number): Promise<void>;
}

export class PrismaEventsRepository implements EventsRepository {
  // Used only for post-commit status updates, which happen after the
  // transaction that created the delivery row has already finished.
  constructor(private readonly prisma: PrismaClient) {}

  async insertEvent(db: Db, accountId: string, type: string, resourceId: string, payload: unknown): Promise<Event> {
    return db.event.create({
      data: { accountId, type, resourceId, payload: payload as never },
    });
  }

  async insertDelivery(db: Db, endpointId: string, eventId: string): Promise<WebhookDelivery> {
    return db.webhookDelivery.create({
      data: { endpointId, eventId, status: "pending" },
    });
  }

  async updateDeliveryStatus(id: string, status: "delivered" | "failed", attempts: number): Promise<void> {
    await this.prisma.webhookDelivery.update({
      where: { uuid: id },
      data: { status, attempts },
    });
  }
}
