export class ValidationError extends Error {}
export class NotFoundError extends Error {}
export class ConflictError extends Error {}
export class UnauthorizedError extends Error {}
export class UnprocessableError extends Error {}

export function statusForError(err: unknown): number | undefined {
  if (err instanceof ValidationError) return 400;
  if (err instanceof UnauthorizedError) return 401;
  if (err instanceof NotFoundError) return 404;
  if (err instanceof ConflictError) return 409;
  if (err instanceof UnprocessableError) return 422;
  return undefined;
}
