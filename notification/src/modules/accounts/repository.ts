import type { Account, ApiKey, PrismaClient } from "@prisma/client";

export interface AccountRepository {
  createAccount(name: string): Promise<Account>;
  createApiKey(accountId: string, keyHash: string): Promise<ApiKey>;
  getAccountByApiKeyHash(keyHash: string): Promise<Account | null>;
  touchApiKeyLastUsed(keyHash: string): Promise<void>;
}

export class PrismaAccountRepository implements AccountRepository {
  constructor(private readonly prisma: PrismaClient) {}

  async createAccount(name: string): Promise<Account> {
    return this.prisma.account.create({ data: { name } });
  }

  async createApiKey(accountId: string, keyHash: string): Promise<ApiKey> {
    return this.prisma.apiKey.create({ data: { accountId, keyHash } });
  }

  async getAccountByApiKeyHash(keyHash: string): Promise<Account | null> {
    const apiKey = await this.prisma.apiKey.findUnique({
      where: { keyHash },
      include: { account: true },
    });
    return apiKey?.account ?? null;
  }

  async touchApiKeyLastUsed(keyHash: string): Promise<void> {
    await this.prisma.apiKey.update({
      where: { keyHash },
      data: { lastUsedAt: new Date() },
    });
  }
}
