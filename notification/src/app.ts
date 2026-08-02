import Fastify, { type FastifyInstance } from "fastify";
import type { Config } from "./config.js";

export function buildApp(config: Config): FastifyInstance {
  const app = Fastify({ logger: true });

  app.get("/health", async () => ({ status: "ok" }));

  void config;

  return app;
}
