import type { Channel, Message, MessageStatus, PrismaClient } from "@prisma/client";
import type { Db } from "../../lib/prisma.js";

export interface CreateMessageInput {
  accountId: string;
  channel: Channel;
  toAddress: string;
  fromAddress: string;
  subject: string | null;
  body: string;
  status: MessageStatus;
}

export interface MessageRepository {
  /** Writes via db so it can participate in the same transaction as the outbox event insert. */
  create(db: Db, input: CreateMessageInput): Promise<Message>;
  updateStatus(db: Db, id: string, status: MessageStatus): Promise<Message>;
  listByAccount(accountId: string): Promise<Message[]>;
  getById(accountId: string, id: string): Promise<Message | null>;
}

export class PrismaMessageRepository implements MessageRepository {
  constructor(private readonly prisma: PrismaClient) {}

  async create(db: Db, input: CreateMessageInput): Promise<Message> {
    return db.message.create({ data: input });
  }

  async updateStatus(db: Db, id: string, status: MessageStatus): Promise<Message> {
    return db.message.update({ where: { uuid: id }, data: { status } });
  }

  async listByAccount(accountId: string): Promise<Message[]> {
    return this.prisma.message.findMany({ where: { accountId }, orderBy: { createdAt: "desc" } });
  }

  async getById(accountId: string, id: string): Promise<Message | null> {
    return this.prisma.message.findFirst({ where: { accountId, uuid: id } });
  }
}
