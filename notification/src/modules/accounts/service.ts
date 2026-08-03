import { randomBytes, createHash } from "node:crypto";
import type { Account } from "@prisma/client";
import type { AccountRepository } from "./repository.js";
import { UnauthorizedError, ValidationError } from "../../lib/errors.js";

export class InvalidApiKeyError extends UnauthorizedError {}

export interface AccountService {
  createAccount(name: string): Promise<{ account: Account; apiKey: string }>;
  authenticateApiKey(rawKey: string): Promise<Account>;
}

export class AccountServiceImpl implements AccountService {
  constructor(private readonly repo: AccountRepository) {}

  async createAccount(name: string): Promise<{ account: Account; apiKey: string }> {
    if (!name || name.trim() === "") {
      throw new ValidationError("name must not be empty");
    }

    const account = await this.repo.createAccount(name);
    const { rawKey, keyHash } = generateApiKey();
    await this.repo.createApiKey(account.uuid, keyHash);

    return { account, apiKey: rawKey };
  }

  async authenticateApiKey(rawKey: string): Promise<Account> {
    if (!rawKey) throw new InvalidApiKeyError("invalid api key");

    const keyHash = hashApiKey(rawKey);
    const account = await this.repo.getAccountByApiKeyHash(keyHash);
    if (!account) throw new InvalidApiKeyError("invalid api key");

    // Best-effort; a failure to record last-used shouldn't fail the request.
    this.repo.touchApiKeyLastUsed(keyHash).catch(() => {});

    return account;
  }
}

function generateApiKey(): { rawKey: string; keyHash: string } {
  const rawKey = `notif_live_${randomBytes(24).toString("hex")}`;
  return { rawKey, keyHash: hashApiKey(rawKey) };
}

function hashApiKey(rawKey: string): string {
  return createHash("sha256").update(rawKey).digest("hex");
}
