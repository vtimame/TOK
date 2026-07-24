import type { AxiosError } from "axios";
import { toast } from "vue-sonner";

type HttpError = {
  status: number;
  error: string;
  message: string;
};

export function useApiError(error: unknown): HttpError {
  const e = (error as AxiosError)?.response?.data as HttpError;
  return {
    status: e?.status || 500,
    error: e?.error || "INTERNAL_SERVER_ERROR",
    message: e?.message || "Unhandled error",
  };
}

export function toastApiError(
  error: unknown,
  messages?: { [status: number]: string | { title: string; description: string } },
): HttpError {
  let title = "Error";
  let description = "Unhandled error";

  const e = useApiError(error);
  const msg = messages?.[e.status];

  if (msg && typeof msg === "string") {
    description = msg;
  } else if (typeof msg === "object") {
    title = msg.title;
    description = msg.description;
  }

  toast(title, { description });
  return e;
}
