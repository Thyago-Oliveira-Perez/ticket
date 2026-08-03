import type { Db } from "../../lib/prisma.js";
import type { EventsRepository } from "./repository.js";
import type { WebhookEndpointRepository } from "../webhookEndpoints/repository.js";
import type { WebhookSender } from "../webhookDelivery/sender.js";

export interface Delivery {
  id: string;
  endpointUrl: string;
  endpointSecret: string;
  eventType: string;
  payload: unknown;
  attempts: number;
}

export interface EventPublisher {
  /**
   * Records that `type` happened to `resourceId` (writing the event and one
   * pending delivery row per active endpoint via db, so they commit
   * atomically with whatever state change db is also part of), and returns
   * the deliveries so the caller can dispatch them once that transaction
   * has committed.
   */
  publish(db: Db, accountId: string, type: string, resourceId: string, payload: unknown): Promise<Delivery[]>;
  /**
   * Attempts HTTP delivery for each delivery, updating its stored status
   * afterward. Safe to call only after the transaction that produced
   * deliveries has committed. Non-blocking: each delivery is dispatched
   * without the caller awaiting it.
   */
  dispatch(deliveries: Delivery[]): void;
}

export class EventPublisherImpl implements EventPublisher {
  constructor(
    private readonly events: EventsRepository,
    private readonly endpoints: WebhookEndpointRepository,
    private readonly sender: WebhookSender,
  ) {}

  async publish(db: Db, accountId: string, type: string, resourceId: string, payload: unknown): Promise<Delivery[]> {
    const event = await this.events.insertEvent(db, accountId, type, resourceId, payload);
    const activeEndpoints = await this.endpoints.listActiveByAccount(accountId);

    const deliveries: Delivery[] = [];
    for (const endpoint of activeEndpoints) {
      const delivery = await this.events.insertDelivery(db, endpoint.uuid, event.uuid);
      deliveries.push({
        id: delivery.uuid,
        endpointUrl: endpoint.url,
        endpointSecret: endpoint.secret,
        eventType: type,
        payload,
        attempts: delivery.attempts,
      });
    }

    return deliveries;
  }

  dispatch(deliveries: Delivery[]): void {
    for (const delivery of deliveries) {
      void this.dispatchOne(delivery);
    }
  }

  private async dispatchOne(delivery: Delivery): Promise<void> {
    let status: "delivered" | "failed" = "delivered";
    try {
      await this.sender.send(delivery.endpointUrl, delivery.endpointSecret, delivery.eventType, delivery.payload);
    } catch (err) {
      status = "failed";
      console.error(`events: delivery ${delivery.id} to ${delivery.endpointUrl} failed:`, err);
    }

    try {
      await this.events.updateDeliveryStatus(delivery.id, status, delivery.attempts + 1);
    } catch (err) {
      console.error(`events: failed to record delivery status for ${delivery.id}:`, err);
    }
  }
}
