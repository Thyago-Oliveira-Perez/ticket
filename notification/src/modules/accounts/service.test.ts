import { test } from "node:test";
import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import type { Account, ApiKey } from "@prisma/client";
import type { AccountRepository } from "./repository.js";
import { AccountServiceImpl, InvalidApiKeyError } from "./service.js";
import { ValidationError } from "../../lib/errors.js";

class FakeAccountRepository implements AccountRepository {
  accounts: Account[] = [];
  apiKeys: ApiKey[] = [];

  async createAccount(name: string): Promise<Account> {
    const account: Account = {
      uuid: randomUUID(),
      name,
      status: "active",
      createdAt: new Date(),
      updatedAt: new Date(),
    };
    this.accounts.push(account);
    return account;
  }

  async createApiKey(accountId: string, keyHash: string): Promise<ApiKey> {
    const apiKey: ApiKey = {
      uuid: randomUUID(),
      accountId,
      keyHash,
      scope: "full_access",
      lastUsedAt: null,
      createdAt: new Date(),
      updatedAt: new Date(),
    };
    this.apiKeys.push(apiKey);
    return apiKey;
  }

  async getAccountByApiKeyHash(keyHash: string): Promise<Account | null> {
    const apiKey = this.apiKeys.find((k) => k.keyHash === keyHash);
    if (!apiKey) return null;
    return this.accounts.find((a) => a.uuid === apiKey.accountId) ?? null;
  }

  async touchApiKeyLastUsed(): Promise<void> {}
}

test("createAccount issues a usable api key", async () => {
  const service = new AccountServiceImpl(new FakeAccountRepository());

  const { account, apiKey } = await service.createAccount("Acme Inc");
  assert.equal(account.name, "Acme Inc");
  assert.match(apiKey, /^notif_live_[0-9a-f]{48}$/);

  const authenticated = await service.authenticateApiKey(apiKey);
  assert.equal(authenticated.uuid, account.uuid);
});

test("createAccount rejects an empty name", async () => {
  const service = new AccountServiceImpl(new FakeAccountRepository());
  await assert.rejects(() => service.createAccount(""), ValidationError);
});

test("authenticateApiKey rejects an unknown key", async () => {
  const service = new AccountServiceImpl(new FakeAccountRepository());
  await assert.rejects(() => service.authenticateApiKey("notif_live_bogus"), InvalidApiKeyError);
});
