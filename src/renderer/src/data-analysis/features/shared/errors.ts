import { ApiClientError } from "../../services/http/client";

export function toErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    const code = error.code || "API_ERROR";
    return `${code}: ${error.message}`;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "unknown error";
}
