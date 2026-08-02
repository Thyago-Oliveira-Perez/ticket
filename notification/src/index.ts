import { execFileSync } from "node:child_process";
import { buildApp } from "./app.js";
import { loadConfig } from "./config.js";

const config = loadConfig();

if (config.dbDsn) {
  execFileSync("npx", ["prisma", "migrate", "deploy"], {
    stdio: "inherit",
    env: { ...process.env, DB_DSN: config.dbDsn },
  });
}

const app = buildApp(config);

app
  .listen({ host: config.addr, port: config.port })
  .catch((err) => {
    app.log.error(err);
    process.exit(1);
  });
