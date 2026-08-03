import type { Message, PrismaClient } from "@prisma/client";
import type { MessageChaosConfig } from "../../config.js";
import type { Db } from "../../lib/prisma.js";
import type { MessageRepository } from "./repository.js";
import type { SuppressionRepository } from "../suppressions/repository.js";
import type { EventPublisher } from "../events/publisher.js";
import { NotFoundError, ValidationError } from "../../lib/errors.js";

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export interface SendMessageInput {
  channel?: string;
  to?: string;
  from?: string;
  subject?: string;
  body?: string;
}

export interface MessageService {
  sendMessage(accountId: string, input: SendMessageInput): Promise<Message>;
  listByAccount(accountId: string): Promise<Message[]>;
  getById(accountId: string, id: string): Promise<Message>;
}

export function serializeMessage(message: Message) {
  return {
    id: message.uuid,
    channel: message.channel,
    to: message.toAddress,
    from: message.fromAddress,
    subject: message.subject,
    body: message.body,
    status: message.status,
    created_at: message.createdAt,
    updated_at: message.updatedAt,
  };
}

export class MessageServiceImpl implements MessageService {
  private readonly rollBounce: () => boolean;

  constructor(
    private readonly prisma: PrismaClient,
    private readonly repo: MessageRepository,
    private readonly suppressions: SuppressionRepository,
    private readonly publisher: EventPublisher,
    private readonly cfg: MessageChaosConfig,
    rollBounce?: () => boolean,
  ) {
    this.rollBounce = rollBounce ?? (() => this.cfg.bounceRate > 0 && Math.random() < this.cfg.bounceRate);
  }

  async sendMessage(accountId: string, input: SendMessageInput): Promise<Message> {
    const validated = validateInput(input);

    const suppressed = await this.suppressions.isSuppressed(accountId, validated.channel, validated.toAddress);
    const status = suppressed ? "suppressed" : "queued";
    const eventType = suppressed ? "message.suppressed" : "message.queued";

    const { message, deliveries } = await this.prisma.$transaction(async (tx) => {
      const created = await this.repo.create(tx, { accountId, status, ...validated });
      const deliveries = await this.publisher.publish(tx, accountId, eventType, created.uuid, serializeMessage(created));
      return { message: created, deliveries };
    });

    this.publisher.dispatch(deliveries);

    if (!suppressed) {
      void this.runLifecycle(accountId, message);
    }

    return message;
  }

  async listByAccount(accountId: string): Promise<Message[]> {
    return this.repo.listByAccount(accountId);
  }

  async getById(accountId: string, id: string): Promise<Message> {
    if (!UUID_RE.test(id)) throw new ValidationError("message id must be a valid UUID");

    const message = await this.repo.getById(accountId, id);
    if (!message) throw new NotFoundError("message not found");
    return message;
  }

  // Runs in the background after sendMessage has already returned the
  // queued message to the caller — mirrors how nubrank's disputes lifecycle
  // simulates provider behavior after the fact, not in response to a
  // caller action.
  private async runLifecycle(accountId: string, message: Message): Promise<void> {
    await sleep(this.cfg.sendDelayMinMs, this.cfg.sendDelayMaxMs);
    await this.transition(accountId, message.uuid, "sent", "message.sent");

    await sleep(this.cfg.deliverDelayMinMs, this.cfg.deliverDelayMaxMs);

    if (this.rollBounce()) {
      // Email hard-bounces permanently suppress the address; an SMS
      // carrier failure doesn't imply the number is bad, so it isn't.
      const finalStatus = message.channel === "email" ? "bounced" : "failed";
      await this.transition(
        accountId,
        message.uuid,
        finalStatus,
        `message.${finalStatus}`,
        finalStatus === "bounced"
          ? (tx) => this.suppressions.upsert(tx, accountId, message.channel, message.toAddress, "bounced")
          : undefined,
      );
    } else {
      await this.transition(accountId, message.uuid, "delivered", "message.delivered");
    }
  }

  private async transition(
    accountId: string,
    id: string,
    status: Message["status"],
    eventType: string,
    sideEffect?: (tx: Db) => Promise<unknown>,
  ): Promise<void> {
    const deliveries = await this.prisma.$transaction(async (tx) => {
      const updated = await this.repo.updateStatus(tx, id, status);
      if (sideEffect) await sideEffect(tx);
      return this.publisher.publish(tx, accountId, eventType, id, serializeMessage(updated));
    });
    this.publisher.dispatch(deliveries);
  }
}

interface ValidatedInput {
  channel: "email" | "sms";
  toAddress: string;
  fromAddress: string;
  subject: string | null;
  body: string;
}

function validateInput(input: SendMessageInput): ValidatedInput {
  if (input.channel !== "email" && input.channel !== "sms") {
    throw new ValidationError('channel must be "email" or "sms"');
  }
  if (!input.to) throw new ValidationError("to must not be empty");
  if (!input.from) throw new ValidationError("from must not be empty");
  if (!input.body) throw new ValidationError("body must not be empty");

  return {
    channel: input.channel,
    toAddress: input.to,
    fromAddress: input.from,
    subject: input.channel === "email" ? (input.subject ?? null) : null,
    body: input.body,
  };
}

function sleep(min: number, max: number): Promise<void> {
  if (max <= 0) return Promise.resolve();
  const delay = max > min ? min + Math.floor(Math.random() * (max - min)) : min;
  return new Promise((resolve) => setTimeout(resolve, delay));
}
