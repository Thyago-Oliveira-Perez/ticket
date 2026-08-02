-- CreateEnum
CREATE TYPE "account_status" AS ENUM ('active', 'suspended');

-- CreateEnum
CREATE TYPE "api_key_scope" AS ENUM ('full_access');

-- CreateEnum
CREATE TYPE "channel" AS ENUM ('email', 'sms');

-- CreateEnum
CREATE TYPE "message_status" AS ENUM ('queued', 'sent', 'delivered', 'bounced', 'failed', 'suppressed');

-- CreateEnum
CREATE TYPE "suppression_reason" AS ENUM ('bounced', 'complained', 'unsubscribed');

-- CreateEnum
CREATE TYPE "webhook_delivery_status" AS ENUM ('pending', 'delivered', 'failed');

-- CreateTable
CREATE TABLE "accounts" (
    "uuid" UUID NOT NULL,
    "name" VARCHAR(255) NOT NULL,
    "status" "account_status" NOT NULL DEFAULT 'active',
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "accounts_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "api_keys" (
    "uuid" UUID NOT NULL,
    "account_id" UUID NOT NULL,
    "key_hash" VARCHAR(64) NOT NULL,
    "scope" "api_key_scope" NOT NULL DEFAULT 'full_access',
    "last_used_at" TIMESTAMPTZ,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "api_keys_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "messages" (
    "uuid" UUID NOT NULL,
    "account_id" UUID NOT NULL,
    "channel" "channel" NOT NULL,
    "to_address" TEXT NOT NULL,
    "from_address" TEXT NOT NULL,
    "subject" TEXT,
    "body" TEXT NOT NULL,
    "status" "message_status" NOT NULL DEFAULT 'queued',
    "provider_message_id" UUID NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "messages_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "suppressions" (
    "uuid" UUID NOT NULL,
    "account_id" UUID NOT NULL,
    "channel" "channel" NOT NULL,
    "address" TEXT NOT NULL,
    "reason" "suppression_reason" NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "suppressions_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "idempotency_keys" (
    "uuid" UUID NOT NULL,
    "account_id" UUID NOT NULL,
    "key" VARCHAR(255) NOT NULL,
    "request_hash" VARCHAR(64) NOT NULL,
    "response_status" INTEGER,
    "response_body" BYTEA,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "idempotency_keys_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "webhook_endpoints" (
    "uuid" UUID NOT NULL,
    "account_id" UUID NOT NULL,
    "url" TEXT NOT NULL,
    "secret" VARCHAR(64) NOT NULL,
    "active" BOOLEAN NOT NULL DEFAULT true,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "webhook_endpoints_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "events" (
    "uuid" UUID NOT NULL,
    "account_id" UUID NOT NULL,
    "type" VARCHAR(64) NOT NULL,
    "resource_id" UUID NOT NULL,
    "payload" JSONB NOT NULL,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "events_pkey" PRIMARY KEY ("uuid")
);

-- CreateTable
CREATE TABLE "webhook_deliveries" (
    "uuid" UUID NOT NULL,
    "endpoint_id" UUID NOT NULL,
    "event_id" UUID NOT NULL,
    "attempts" INTEGER NOT NULL DEFAULT 0,
    "status" "webhook_delivery_status" NOT NULL DEFAULT 'pending',
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "webhook_deliveries_pkey" PRIMARY KEY ("uuid")
);

-- CreateIndex
CREATE UNIQUE INDEX "api_keys_key_hash_key" ON "api_keys"("key_hash");

-- CreateIndex
CREATE INDEX "api_keys_account_id_idx" ON "api_keys"("account_id");

-- CreateIndex
CREATE INDEX "messages_account_id_idx" ON "messages"("account_id");

-- CreateIndex
CREATE UNIQUE INDEX "suppressions_account_id_channel_address_key" ON "suppressions"("account_id", "channel", "address");

-- CreateIndex
CREATE UNIQUE INDEX "idempotency_keys_account_id_key_key" ON "idempotency_keys"("account_id", "key");

-- CreateIndex
CREATE INDEX "webhook_endpoints_account_id_idx" ON "webhook_endpoints"("account_id");

-- CreateIndex
CREATE INDEX "events_account_id_idx" ON "events"("account_id");

-- CreateIndex
CREATE INDEX "events_resource_id_idx" ON "events"("resource_id");

-- CreateIndex
CREATE INDEX "webhook_deliveries_endpoint_id_idx" ON "webhook_deliveries"("endpoint_id");

-- CreateIndex
CREATE INDEX "webhook_deliveries_event_id_idx" ON "webhook_deliveries"("event_id");

-- AddForeignKey
ALTER TABLE "api_keys" ADD CONSTRAINT "api_keys_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "accounts"("uuid") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "messages" ADD CONSTRAINT "messages_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "accounts"("uuid") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "suppressions" ADD CONSTRAINT "suppressions_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "accounts"("uuid") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "idempotency_keys" ADD CONSTRAINT "idempotency_keys_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "accounts"("uuid") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "webhook_endpoints" ADD CONSTRAINT "webhook_endpoints_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "accounts"("uuid") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "events" ADD CONSTRAINT "events_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "accounts"("uuid") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "webhook_deliveries" ADD CONSTRAINT "webhook_deliveries_endpoint_id_fkey" FOREIGN KEY ("endpoint_id") REFERENCES "webhook_endpoints"("uuid") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "webhook_deliveries" ADD CONSTRAINT "webhook_deliveries_event_id_fkey" FOREIGN KEY ("event_id") REFERENCES "events"("uuid") ON DELETE RESTRICT ON UPDATE CASCADE;
