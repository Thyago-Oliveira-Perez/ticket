import type { Suppression } from "@prisma/client";
import type { SuppressionRepository } from "./repository.js";

export interface SuppressionService {
  listByAccount(accountId: string): Promise<Suppression[]>;
}

export class SuppressionServiceImpl implements SuppressionService {
  constructor(private readonly repo: SuppressionRepository) {}

  async listByAccount(accountId: string): Promise<Suppression[]> {
    return this.repo.listByAccount(accountId);
  }
}
