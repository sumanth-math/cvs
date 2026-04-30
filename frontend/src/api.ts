import type { APIErrorResponse, BucketRequestPayload, BucketResult } from "./types";

const configuredBaseURL = import.meta.env.VITE_API_BASE_URL?.trim().replace(/\/$/, "") ?? "";

export async function createBucket(payload: BucketRequestPayload, signal?: AbortSignal): Promise<BucketResult> {
  const response = await fetch(`${configuredBaseURL}/v1/s3-buckets`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: JSON.stringify(payload),
    signal
  });

  const data = await response.json().catch(() => undefined);

  if (!response.ok) {
    const apiError = data as APIErrorResponse | undefined;
    const message = apiError?.message || `Request failed with status ${response.status}`;
    const error = new Error(message) as Error & { details?: APIErrorResponse };
    error.details = apiError;
    throw error;
  }

  return data as BucketResult;
}
